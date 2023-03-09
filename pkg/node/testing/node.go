package testing

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	messagev1 "github.com/xmtp/proto/v3/go/message_api/v1"
	"github.com/xmtp/xmtpd/pkg/api/client"
	"github.com/xmtp/xmtpd/pkg/context"
	"github.com/xmtp/xmtpd/pkg/node"
	memstore "github.com/xmtp/xmtpd/pkg/store/mem"
	test "github.com/xmtp/xmtpd/pkg/testing"
	"github.com/xmtp/xmtpd/pkg/zap"
	"google.golang.org/protobuf/proto"
)

type testNode struct {
	*node.Node
	name string

	client client.Client
	ctx    context.Context
}

func NewTestNode(t *testing.T) *testNode {
	return NewTestNodeWithName(t, "")
}

func NewTestNodeWithName(t *testing.T, name string) *testNode {
	t.Helper()
	ctx := test.NewContext(t)
	if name != "" {
		ctx = context.WithLogger(ctx, ctx.Logger().Named(name))
	}
	return NewTestNodeWithNameAndStore(t, ctx, name, memstore.NewNodeStore(ctx))
}

func NewTestNodeWithNameAndStore(t *testing.T, ctx context.Context, name string, store node.NodeStore) *testNode {
	t.Helper()

	node, err := node.New(ctx, store, &node.Options{
		OpenTelemetry: node.OpenTelemetryOptions{
			Disable: true, // otel fails race detector atm
			// CollectorAddress: "localhost",
			// CollectorPort:    4317,
		},
	})
	require.NoError(t, err)

	client := client.NewHTTPClient(ctx.Logger(), fmt.Sprintf("http://localhost:%d", node.APIHTTPListenPort()), "test", name)

	return &testNode{
		Node:   node,
		name:   name,
		client: client,
		ctx:    ctx,
	}
}

func (n *testNode) Close() error {
	n.ctx.Close()
	n.Node.Close()
	return nil
}

func (n *testNode) Connect(t *testing.T, to *testNode) {
	t.Helper()

	err := n.Node.Connect(n.ctx, to.Address())
	require.NoError(t, err)
	log := n.ctx.Logger()

	// Wait for peers to be connected and grafted to the pubsub topic.
	// See https://github.com/libp2p/go-libp2p-pubsub/issues/331
	totalTimeout := 5 * time.Second
	if os.Getenv("CI") == "true" {
		totalTimeout = 10 * time.Second
	}
	retryTimeout := totalTimeout / 10
	ticker := time.NewTicker(retryTimeout)
	defer ticker.Stop()
	attempt := 1
	var connected bool
	ctx := context.WithTimeout(n.ctx, totalTimeout)
	defer ctx.Close()
	topic := "sync-" + test.RandomStringLower(13)
syncLoop:
	for {
		select {
		case <-ctx.Done():
			log.Info("context closed", zap.Error(ctx.Err()))
			break syncLoop
		case <-ticker.C:
			sentEnv := newRandomEnvelope(topic, attempt)
			_, err := n.client.Publish(n.ctx, &messagev1.PublishRequest{
				Envelopes: []*messagev1.Envelope{sentEnv},
			})
			require.NoError(t, err)

			func() {
				queryTicker := time.NewTicker(retryTimeout / 5)
				defer queryTicker.Stop()
				queryCtx := context.WithTimeout(ctx, retryTimeout)
				defer queryCtx.Close()
				for {
					select {
					case <-queryCtx.Done():
						return
					case <-queryTicker.C:
						res, err := to.client.Query(n.ctx, &messagev1.QueryRequest{
							ContentTopics: []string{topic},
							PagingInfo: &messagev1.PagingInfo{
								Direction: messagev1.SortDirection_SORT_DIRECTION_DESCENDING,
								Limit:     1,
							},
						})
						if err != nil {
							continue
						}
						if len(res.Envelopes) > 0 && proto.Equal(sentEnv, res.Envelopes[len(res.Envelopes)-1]) {
							connected = true
							return
						}
					}
				}
			}()
			if connected {
				break syncLoop
			}

			log.Debug("waiting for p2p connectivity sync message", zap.Int("attempt", attempt))
			attempt++
		}
	}
	require.True(t, connected, fmt.Sprintf("node %s failed to connect to node %s", n.name, to.name))
}

func (n *testNode) PublishRandom(t *testing.T, topic string, count int) []*messagev1.Envelope {
	t.Helper()
	envs := make([]*messagev1.Envelope, count)
	for i := 0; i < count; i++ {
		env := newRandomEnvelope(topic, rand.Intn(100))
		res, err := n.client.Publish(n.ctx, &messagev1.PublishRequest{
			Envelopes: []*messagev1.Envelope{env},
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		envs[i] = env
	}
	return envs
}

func (n *testNode) Subscribe(t *testing.T, topic string) *testSubscriber {
	sub := &testSubscriber{
		Topic: topic,
	}
	stream, err := n.client.Subscribe(n.ctx, &messagev1.SubscribeRequest{
		ContentTopics: []string{topic},
	})
	require.NoError(t, err)
	sub.stream = stream
	go func() {
		for {
			env, err := sub.stream.Next(n.ctx)
			if err == context.Canceled {
				return
			}
			require.NoError(t, err)
			func() {
				sub.Lock()
				defer sub.Unlock()
				sub.envs = append(sub.envs, env)
			}()
		}
	}()
	return sub
}

func (n *testNode) RequireStoredEvents(t *testing.T, topic string, expected []*messagev1.Envelope) {
	t.Helper()
	res, err := n.client.Query(n.ctx, &messagev1.QueryRequest{
		ContentTopics: []string{topic},
	})
	require.NoError(t, err)
	require.Len(t, res.Envelopes, len(expected))
	requireEnvelopesEqual(t, expected, res.Envelopes)
}

func requireEnvelopesEqual(t *testing.T, actual, expected []*messagev1.Envelope) {
	t.Helper()
	expected = expected[:]
	sort.Slice(expected, func(i, j int) bool {
		d := int(expected[i].TimestampNs) - int(expected[j].TimestampNs)
		if d != 0 {
			return d < 0
		}
		return bytes.Compare(expected[i].Message, expected[j].Message) < 0
	})
	actual = actual[:]
	sort.Slice(actual, func(i, j int) bool {
		d := int(actual[i].TimestampNs) - int(actual[j].TimestampNs)
		if d != 0 {
			return d < 0
		}
		return bytes.Compare(actual[i].Message, actual[j].Message) < 0
	})
	test.RequireProtoEqual(t, expected, actual)
}

func newRandomEnvelope(topic string, timestampNs int) *messagev1.Envelope {
	return &messagev1.Envelope{
		ContentTopic: topic,
		TimestampNs:  uint64(timestampNs),
		Message:      []byte("msg-" + test.RandomString(13)),
	}
}