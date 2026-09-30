package flow

import (
	"errors"
	"os"
	"path/filepath"
)

var (
	ErrFlowNotFound         = errors.New("flow: flow not found")
	ErrNodeNotFound         = errors.New("flow: node not found")
	ErrNodeNotImageMarkable = errors.New("flow: node does not carry an image mark")
)

// markImageConfig is a source connector config whose feed mark can be an
// uploaded image, letting this package set the mark without naming a connector.
type markImageConfig interface {
	MarkImage() string
	SetMarkImage(hash string)
}

// SetSourceImage sets (hash != "") or clears (hash == "") source node nodeID's
// feed-mark image in flow flowID, saves the flow, and returns the reloaded flow.
// It reads from disk rather than the cached snapshot, so a concurrent external
// edit is not reverted.
func (s *FlowStore) SetSourceImage(flowID, nodeID, hash string) (Flow, error) {
	if !validSlug(flowID) {
		return Flow{}, ErrFlowNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, flowID+".yaml")
	f, _, err := LoadFlow(path, s.refs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Flow{}, ErrFlowNotFound
		}
		return Flow{}, err
	}
	cfg, err := markImageNodeConfig(f, nodeID)
	if err != nil {
		return Flow{}, err
	}
	cfg.SetMarkImage(hash)
	if err := SaveFlow(path, f); err != nil {
		return Flow{}, err
	}
	if err := s.reloadLocked(); err != nil {
		return Flow{}, err
	}
	return s.flows[flowID], nil
}

// SourceImageRef returns source node nodeID's feed-mark image hash, or "" when
// it has none.
func (s *FlowStore) SourceImageRef(flowID, nodeID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	f, ok := s.flows[flowID]
	if !ok {
		return "", ErrFlowNotFound
	}
	cfg, err := markImageNodeConfig(f, nodeID)
	if err != nil {
		return "", err
	}
	return cfg.MarkImage(), nil
}

// markImageNodeConfig returns nodeID's config as a markImageConfig, or a
// sentinel error when the node is missing or carries no image mark.
func markImageNodeConfig(f Flow, nodeID string) (markImageConfig, error) {
	for i := range f.Nodes {
		if f.Nodes[i].ID != nodeID {
			continue
		}
		sc, ok := f.Nodes[i].Config.(*SourceConfig)
		if !ok {
			return nil, ErrNodeNotImageMarkable
		}
		cfg, ok := sc.Connector().(markImageConfig)
		if !ok {
			return nil, ErrNodeNotImageMarkable
		}
		return cfg, nil
	}
	return nil, ErrNodeNotFound
}
