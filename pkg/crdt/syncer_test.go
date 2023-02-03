package crdt

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	mh "github.com/multiformats/go-multihash"
	"github.com/xmtp/xmtpd/pkg/zap"
)

func Test_BasicSyncing(t *testing.T) {
	// 3 nodes, one topic "t0"
	net := newNetwork(t, 3, 1)
	defer net.Close()
	net.Publish(t, 0, t0, "hi")
	net.Publish(t, 1, t0, "hi back")
	// wait for things to settle
	net.AssertEventuallyConsistent(t, time.Second)
	// suspend broadcasts to n1/t0 and publish few things
	net.WithSuspendedTopic(t, 1, t0, func(n *Node) {
		net.Publish(t, 2, t0, "oh hello")
		net.Publish(t, 2, t0, "how goes")
		net.Publish(t, 1, t0, "how are you")
	})
	// wait for things to settle but ignore n1
	// because it needs a new broadcast to trigger syncing.
	net.AssertEventuallyConsistent(t, time.Second, 1)
	net.Publish(t, 0, t0, "not bad")
	net.AssertEventuallyConsistent(t, time.Second)
}

// In-memory syncer that implements fetching by
// reaching directly into a random Node's store.
type randomSyncer struct {
	sync.RWMutex
	nodes []*Node
}

func newRandomSyncer() *randomSyncer {
	return &randomSyncer{}
}

func (s *randomSyncer) AddNode(n *Node) {
	s.Lock()
	defer s.Unlock()
	s.nodes = append(s.nodes, n)
}

func (s *randomSyncer) GetRandomNode() *Node {
	s.RLock()
	defer s.RUnlock()
	return s.nodes[rand.Intn(len(s.nodes))]
}

func (s *randomSyncer) NewTopic(name string, n *Node) TopicSyncer {
	return &randomTopicSyncer{
		randomSyncer: s,
		node:         n,
		topic:        name,
		log:          n.log.Named(name),
	}
}

type randomTopicSyncer struct {
	*randomSyncer
	node  *Node
	topic string
	log   *zap.Logger
}

func (s *randomTopicSyncer) Fetch(cids []mh.Multihash) (results []*Event, err error) {
	node := s.GetRandomNode()
	s.log.Debug("fetching", zap.Cids("cids", cids...))
	for _, cid := range cids {
		ev, err := node.Get(s.topic, cid)
		if err != nil {
			return nil, err
		}
		results = append(results, ev)
	}
	return results, nil
}
