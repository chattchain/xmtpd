package memstore_test

import (
	"context"
	"testing"

	memstore "github.com/xmtp/xmtpd/pkg/crdt/stores/mem"
	crdttest "github.com/xmtp/xmtpd/pkg/crdt/testing"
	test "github.com/xmtp/xmtpd/pkg/testing"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	log := test.NewLogger(t)

	crdttest.RunStoreEventTests(t, func(t *testing.T) *crdttest.TestStore {
		store := memstore.New(log)
		return crdttest.NewTestStore(ctx, log, store)
	})

	crdttest.RunStoreQueryTests(t, func(t *testing.T) *crdttest.TestStore {
		s := memstore.New(test.NewLogger(t))
		return crdttest.NewTestStore(ctx, log, s)
	})
}