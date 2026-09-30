// Package gitea is the Gitea source connector: the config a gitea source node
// carries, the descriptor declaring what the connector supports, and the
// instance the producer drains on a tick. It covers Forgejo too — the fork
// serves the same /api/v1 surface, and nothing here branches on which server
// answered.
package gitea

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// SourceKind is the inbox source_kind Gitea observations carry.
const SourceKind = "gitea"

// Descriptor declares the connector.
//
// It claims no CapBatchPrefetch: Gitea has no GraphQL endpoint and no
// multi-query REST call, so there is nothing a pre-pass could batch — every
// search is its own request whether it is issued early or on the node's own
// drain.
var Descriptor = connector.Descriptor{
	Type:      "sources.gitea",
	Title:     "Gitea source",
	Provider:  Provider,
	Mode:      connector.ModePull,
	Stability: connector.Stable,
	Capabilities: connector.CapClassify |
		connector.CapConfirmAbsence,
	NewConfig: func() connector.Config { return &Config{} },
}

// NewFactory builds the instance half of the declaration over the live fetch
// path. The node's credential is resolved here, at construction: nodes on the
// same account share one fetcher — and so one cooldown — while nodes on
// different accounts cannot see each other's items.
func NewFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*Config)
			if !ok {
				return connector.Instance{}, fmt.Errorf("gitea source %q: config is %T, want *gitea.Config", node.ID(), cfg)
			}
			ref, err := config.CredentialRef()
			if err != nil {
				return connector.Instance{}, fmt.Errorf("gitea source %q: %w", node.ID(), err)
			}

			fetcher := fetchers.For(ref)

			return connector.Instance{
				Type: Descriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:  node.FlowID,
					SourceKind: SourceKind,
					// The account distinguishes several Gitea sources within one
					// flow, which is what SourceScope is for. It is
					// host-qualified, so two instances never collide.
					SourceScope: ref.Account,
					Policy:      node.Policy,
				},
				Pull:       &source{fetcher: fetcher, config: config, id: node.ID(), topic: node.Topic()},
				Classifier: classifier{},
				// Per instance rather than per factory: the absence confirmer
				// fetches, so it has to fetch as the same account the source did.
				Absence:     &absenceConfirmer{fetcher: fetcher},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
	}
}

// source produces one Gitea source node's current items.
type source struct {
	fetcher *fetcher
	config  *Config
	id      string
	topic   string
}

var _ connector.PullSource = (*source)(nil)

// Produce emits one message per current item of the source, JSON-encoding Item
// as the payload. Key is the item's stable id, used to skip unchanged source
// values within the topic.
//
// The whole result is fetched before the first emit, so a failing fetch cannot
// half-succeed into an authoritative snapshot that archives everything it did
// not reach.
func (s *source) Produce(ctx context.Context, emit func(models.Msg) error) error {
	var (
		items []Item
		err   error
	)
	switch s.config.Kind {
	case KindSearch:
		items, err = s.fetcher.Search(ctx, s.config)
	case KindNotifications:
		items, err = s.fetcher.Notifications(ctx, s.config.effectiveLimit())
	default:
		return fmt.Errorf("gitea source %q: unknown kind %q", s.id, s.config.Kind)
	}
	if err != nil {
		return fmt.Errorf("gitea source %q: %w", s.id, err)
	}

	for _, item := range items {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("gitea source %q: encoding item %q: %w", s.id, item.ID, err)
		}
		msg := models.Msg{Key: item.ID, Topic: s.topic, Payload: payload, SourceKind: SourceKind}
		if err := emit(msg); err != nil {
			return err
		}
	}
	return nil
}
