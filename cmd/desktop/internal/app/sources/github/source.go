// Package github is the GitHub source connector: the config a github source
// node carries, the descriptor declaring what the connector supports, and the
// instance the producer drains on a tick.
package github

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// SourceKind is the inbox source_kind GitHub observations carry.
const SourceKind = "github"

// Descriptor declares the connector. Everything it says is static — the
// registry holds it as package data, and the flow package derives a node type
// from it — while anything needing a live GitHub fetcher goes through
// NewFactory instead.
//
// The capabilities are the declaration that used to be three type assertions
// in the producer. A missed assertion silently downgraded a GitHub source to
// generic ingestion: no classifier, no absence confirmation, no batched
// prefetch. Declaring them means a connector that promises one and does not
// wire it fails a test instead.
var Descriptor = connector.Descriptor{
	Type:      "sources.github",
	Title:     "GitHub source",
	Provider:  Provider,
	Mode:      connector.ModePull,
	Stability: connector.Stable,
	Capabilities: connector.CapClassify |
		connector.CapConfirmAbsence |
		connector.CapBatchPrefetch,
	NewConfig: func() connector.Config { return &Config{} },
}

// NewFactory builds the instance half of the declaration over the live GitHub
// fetch path. The node's credential is resolved here, at construction: two
// nodes on the same account share one fetcher — and therefore one API request
// for identical fetch config, since LiveProvider keys its cache on
// kind+query+limit rather than on id — while nodes on different accounts get
// separate fetchers and cannot see each other's items.
func NewFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*Config)
			if !ok {
				return connector.Instance{}, fmt.Errorf("github source %q: config is %T, want *github.Config", node.ID(), cfg)
			}
			ref, err := config.CredentialRef()
			if err != nil {
				return connector.Instance{}, fmt.Errorf("github source %q: %w", node.ID(), err)
			}

			live := fetchers.For(ref)

			return connector.Instance{
				Type: Descriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:  node.FlowID,
					SourceKind: SourceKind,
					// The account distinguishes several GitHub sources within
					// one flow, which is what SourceScope is for.
					SourceScope: ref.Account,
					Policy:      node.Policy,
				},
				Pull:       &source{live: live, def: sourceDef(node, config), topic: node.Topic()},
				Classifier: classifier{},
				// Per instance rather than per factory: the absence confirmer
				// fetches, so it has to fetch as the same account the source did.
				Absence:     &absenceConfirmer{live: live},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
		Prefetch: func(ctx context.Context, instances []connector.Instance) error {
			// Bucketed by account: a batched search is one API request on one
			// token, so instances on different accounts cannot share one.
			byRef := map[credentials.Ref][]feed.SourceDef{}
			for _, inst := range instances {
				config, ok := inst.Config.(*Config)
				if !ok || config.Kind != KindSearch {
					continue
				}
				ref, err := config.CredentialRef()
				if err != nil {
					continue
				}
				byRef[ref] = append(byRef[ref], sourceDef(inst.Node, config))
			}

			for ref, defs := range byRef {
				if err := fetchers.For(ref).PrefetchSearch(ctx, defs); err != nil {
					return fmt.Errorf("github prefetch for %s: %w", ref, err)
				}
			}
			return nil
		},
	}
}

// sourceDef is the fetch request one node's config asks for. The id is the
// flow-qualified source id; LiveProvider caches on kind+query+limit, so the
// id only identifies the request in logs.
func sourceDef(node connector.Node, cfg *Config) feed.SourceDef {
	return feed.SourceDef{ID: node.ID(), Kind: cfg.Kind, Query: cfg.Query, Limit: cfg.Limit}
}

// source produces one GitHub source node's current items. It does not fetch
// GitHub itself: it delegates to feed.LiveProvider.SourceItems, the same
// coalesced, cached, conditional-request fetch path the desktop feed uses, so
// the pipeline gains a producer without a second implementation of GitHub
// fetching to keep in sync.
type source struct {
	live  *feed.LiveProvider
	def   feed.SourceDef
	topic string
}

var _ connector.PullSource = (*source)(nil)

// Produce emits one message per current item of the source, JSON-encoding
// feed.Item as the payload. Key is the item's stable id, used to skip
// unchanged source values within the topic.
func (s *source) Produce(ctx context.Context, emit func(models.Msg) error) error {
	items, err := s.live.SourceItems(ctx, s.def)
	if err != nil {
		return fmt.Errorf("github source %q: fetching: %w", s.def.ID, err)
	}

	for _, item := range items {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("github source %q: encoding item %q: %w", s.def.ID, item.ID, err)
		}
		msg := models.Msg{Key: item.ID, Topic: s.topic, Payload: payload, SourceKind: SourceKind}
		if err := emit(msg); err != nil {
			return err
		}
	}
	return nil
}
