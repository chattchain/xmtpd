package blockchain

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Construct a raw blockchain listener that can be used to listen for events across many contract event types
type LogStreamBuilder interface {
	ListenForContractEvent(
		fromBlock uint64,
		contractAddress common.Address,
		topic common.Hash,
	) <-chan types.Log
	Build() (LogStreamer, error)
}

type LogStreamer interface {
	Start(ctx context.Context) error
}

type ChainClient interface {
	ethereum.BlockNumberReader
	ethereum.LogFilterer
	ethereum.ChainIDReader
}
