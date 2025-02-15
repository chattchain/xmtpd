package registry

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/xmtp/xmtpd/pkg/abis"
	"github.com/xmtp/xmtpd/pkg/config"
	"go.uber.org/zap"
)

const (
	CONTRACT_CALL_TIMEOUT = 10 * time.Second
)

/*
*
The SmartContractRegistry notifies listeners of changes to the nodes by polling the contract
and diffing the returned node list with what is currently in memory.

This allows it to operate statelessly and not require a database, with a trade-off for latency.

Given how infrequently this list changes, that trade-off seems acceptable.
*/
type SmartContractRegistry struct {
	ctx      context.Context
	contract NodesContract
	logger   *zap.Logger
	// How frequently to poll the smart contract
	refreshInterval time.Duration
	// Mapping of nodes from ID -> Node
	nodes      map[uint16]Node
	nodesMutex sync.RWMutex
	// Notifiers for new nodes and changed nodes
	newNodesNotifier          *notifier[[]Node]
	changedNodeNotifiers      map[uint16]*notifier[Node]
	changedNodeNotifiersMutex sync.RWMutex
}

func NewSmartContractRegistry(
	ethclient bind.ContractCaller,
	logger *zap.Logger,
	options config.ContractsOptions,
) (*SmartContractRegistry, error) {
	contract, err := abis.NewNodesCaller(
		common.HexToAddress(options.NodesContractAddress),
		ethclient,
	)

	if err != nil {
		return nil, err
	}

	return &SmartContractRegistry{
		contract:             contract,
		refreshInterval:      options.RefreshInterval,
		logger:               logger.Named("smartContractRegistry"),
		newNodesNotifier:     newNotifier[[]Node](),
		nodes:                make(map[uint16]Node),
		changedNodeNotifiers: make(map[uint16]*notifier[Node]),
	}, nil
}

/*
*
Loads the initial state from the contract and starts a background refresh loop.

To stop refreshing callers should cancel the context
*
*/
func (s *SmartContractRegistry) Start(ctx context.Context) error {
	s.ctx = ctx
	// If we can't load the data at least once, fail to start the service
	if err := s.refreshData(); err != nil {
		return err
	}

	go s.refreshLoop()

	return nil
}

func (s *SmartContractRegistry) OnNewNodes() (<-chan []Node, CancelSubscription) {
	return s.newNodesNotifier.register()
}

func (s *SmartContractRegistry) OnChangedNode(
	nodeId uint16,
) (<-chan Node, CancelSubscription) {
	s.changedNodeNotifiersMutex.Lock()
	defer s.changedNodeNotifiersMutex.Unlock()

	notifier, ok := s.changedNodeNotifiers[nodeId]
	if !ok {
		notifier = newNotifier[Node]()
		s.changedNodeNotifiers[nodeId] = notifier
	}
	return notifier.register()
}

func (s *SmartContractRegistry) GetNodes() ([]Node, error) {
	s.nodesMutex.RLock()
	defer s.nodesMutex.RUnlock()

	nodes := make([]Node, len(s.nodes))
	for idx, node := range s.nodes {
		nodes[idx] = node
	}
	return nodes, nil
}

func (s *SmartContractRegistry) refreshLoop() {
	ticker := time.NewTicker(s.refreshInterval)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.refreshData(); err != nil {
				s.logger.Error("Failed to refresh data", zap.Error(err))
			}
		}
	}
}

func (s *SmartContractRegistry) refreshData() error {
	fromContract, err := s.loadFromContract()
	if err != nil {
		return err
	}

	newNodes := []Node{}
	for _, node := range fromContract {
		existingValue, ok := s.nodes[node.NodeID]
		if !ok {
			// New node found
			newNodes = append(newNodes, node)
		} else if !node.Equals(existingValue) {
			s.processChangedNode(node)
		}
	}

	if len(newNodes) > 0 {
		s.processNewNodes(newNodes)
	}

	return nil
}

func (s *SmartContractRegistry) processNewNodes(nodes []Node) {
	s.logger.Info("processing new nodes", zap.Int("count", len(nodes)), zap.Any("nodes", nodes))
	s.newNodesNotifier.trigger(nodes)

	s.nodesMutex.Lock()
	defer s.nodesMutex.Unlock()
	for _, node := range nodes {
		s.nodes[node.NodeID] = node
	}
}

func (s *SmartContractRegistry) processChangedNode(node Node) {
	s.nodesMutex.Lock()
	defer s.nodesMutex.Unlock()
	s.changedNodeNotifiersMutex.RLock()
	defer s.changedNodeNotifiersMutex.RUnlock()

	s.nodes[node.NodeID] = node
	s.logger.Info("processing changed node", zap.Any("node", node))
	if registry, ok := s.changedNodeNotifiers[node.NodeID]; ok {
		registry.trigger(node)
	}
}

func (s *SmartContractRegistry) loadFromContract() ([]Node, error) {
	ctx, cancel := context.WithTimeout(s.ctx, CONTRACT_CALL_TIMEOUT)
	defer cancel()
	nodes, err := s.contract.AllNodes(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, err
	}
	out := make([]Node, len(nodes))
	for idx, node := range nodes {
		out[idx] = convertNode(node)
	}

	return out, nil
}

func (s *SmartContractRegistry) SetContractForTest(contract NodesContract) {
	s.contract = contract
}

func convertNode(rawNode abis.NodesNodeWithId) Node {
	// Unmarshal the signing key.
	// If invalid, mark the config as being invalid as well. Clients should treat the
	// node as unhealthy in this case
	signingKey, err := crypto.UnmarshalPubkey(rawNode.Node.SigningKeyPub)
	isValidConfig := err == nil

	httpAddress := rawNode.Node.HttpAddress

	// Ensure the httpAddress is well formed
	if !strings.HasPrefix(httpAddress, "https://") && !strings.HasPrefix(httpAddress, "http://") {
		isValidConfig = false
	}

	return Node{
		NodeID:        rawNode.NodeId,
		SigningKey:    signingKey,
		HttpAddress:   httpAddress,
		IsHealthy:     rawNode.Node.IsHealthy,
		IsValidConfig: isValidConfig,
	}
}
