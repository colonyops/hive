package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/profileimg"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sourcemark"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// FlowsService owns the flow definitions: the listing the picker renders,
// the CRUD the editor drives, the layout files the canvas persists, and each
// profile's sidebar-rail avatar.
type FlowsService struct {
	flows    *flow.FlowStore
	stores   *stores.Stores
	creds    credentials.Store
	images   *profileimg.Store
	marks    *sourcemark.Store
	scripts  *runtime.ScriptRegistry
	settings *settings.Store
	events   *events.Bus
}

type FlowsDeps struct {
	Flows *flow.FlowStore
	// Profile deletion spans aggregates in one transaction.
	Stores   *stores.Stores
	Creds    credentials.Store
	Images   *profileimg.Store
	Marks    *sourcemark.Store
	Scripts  *runtime.ScriptRegistry
	Settings *settings.Store
	Events   *events.Bus
}

func newFlowsService(d FlowsDeps) *FlowsService {
	return &FlowsService{
		flows:    d.Flows,
		stores:   d.Stores,
		creds:    d.Creds,
		images:   d.Images,
		marks:    d.Marks,
		scripts:  d.Scripts,
		settings: d.Settings,
		events:   d.Events,
	}
}

// seedCredential is the account a starter graph fetches as, or "" when there
// is no unambiguous one.
//
// Exactly one connected account is the case this resolves. Zero has nothing
// to seed with; several is not guessed at, because seeding a whole profile
// against the wrong account is not something a user would notice until the
// feed was already wrong. Neither is an error — a profile is the thing that
// exists without any credential, so both simply mean "unseeded".
func (s *FlowsService) seedCredential() (string, error) {
	refs, err := credentials.ListProvider(s.creds, ghsource.Provider)
	if err != nil {
		return "", Wrap(err, KindInternal, "reading credentials")
	}
	if len(refs) != 1 {
		return "", nil
	}
	return refs[0].String(), nil
}

func (s *FlowsService) notifyUpdated(ctx context.Context) {
	s.events.Publish(ctx, events.FlowsUpdated{Reason: "save"})
}

// requireProfile resolves an id to its parsed flow, or reports KindNotFound.
// Every method taking a profile id goes through this or requireProfileExists,
// so a typo is answered the same way everywhere rather than by whatever the
// underlying write happened to do with a missing file.
func (s *FlowsService) requireProfile(id string) (flow.Flow, error) {
	f, ok := s.flows.Get(id)
	if !ok {
		return flow.Flow{}, Errorf(KindNotFound, "profile %q not found", id)
	}
	return f, nil
}

// requireProfileExists is requireProfile for the operations that do not need
// the graph. A profile whose file does not parse still exists — it lists with
// valid=false, and renaming or deleting it is how that gets resolved — so
// those operations must not be gated on it loading.
func (s *FlowsService) requireProfileExists(id string) error {
	if !s.flows.Exists(id) {
		return Errorf(KindNotFound, "profile %q not found", id)
	}
	return nil
}

// requireDeletable is requireProfileExists widened by whatever the last delete
// may have left behind. See Delete.
func (s *FlowsService) requireDeletable(ctx context.Context, id string) error {
	if s.flows.Exists(id) {
		return nil
	}
	items, err := s.stores.InboxItems.ListAll(ctx, id, 1)
	if err != nil {
		return Wrap(err, KindInternal, "reading inbox rows for profile %q", id)
	}
	if len(items) > 0 {
		return nil
	}
	return Errorf(KindNotFound, "profile %q not found", id)
}

// Statuses returns one status per flow file — valid and invalid alike — so a
// broken file shows up with its error instead of silently vanishing.
func (s *FlowsService) Statuses(context.Context) []flow.FlowStatus {
	return s.flows.Statuses()
}

// DefaultProfileName names the profile the app creates so that one always
// exists. First run never asks for a name: a profile is the container the
// starter feeds land in, and there is nothing to say about it before there is
// anything in it.
const DefaultProfileName = "Default"

// EnsureProfile creates the default profile when the flows directory holds no
// flow file at all. It runs at startup and after a delete, the two moments
// the set can be empty, so neither the rail nor first run ever faces an app
// with no profile. A broken file counts as a profile: it is listed with its
// error, and writing a Default beside it would hide that.
func (s *FlowsService) EnsureProfile(ctx context.Context) error {
	if len(s.flows.Statuses()) > 0 {
		return nil
	}
	_, err := s.Create(ctx, DefaultProfileName)
	return err
}

// Create makes a new flow named name, seeded with the starter graph when
// exactly one GitHub account is connected and empty otherwise.
//
// Creating is never refused for want of a credential. First run has the
// default profile before it offers to connect anything, so the unseeded case
// is the expected one there, not a failure — SeedStarter is what fills it in
// once the account exists.
func (s *FlowsService) Create(ctx context.Context, name string) (flow.Flow, error) {
	credential, err := s.seedCredential()
	if err != nil {
		return flow.Flow{}, err
	}
	var seed flow.Seed
	if credential != "" {
		seed = starterSeed(credential)
	}
	f, err := s.flows.Create(name, seed)
	if err != nil {
		return flow.Flow{}, Wrap(err, KindInvalid, "creating profile %q", name)
	}
	s.notifyUpdated(ctx)
	return f, nil
}

// SeedStarter fills an empty profile with the starter graph, fetching as
// the one connected GitHub account. First run calls it when the account it
// offered to connect finally exists: the profile was created a step
// earlier, before there was anything to seed it with.
//
// A profile that already has nodes is refused rather than appended to.
// Appending a second starter graph onto a graph someone has since edited is
// not a mistake they can undo.
func (s *FlowsService) SeedStarter(ctx context.Context, id string) (flow.Flow, error) {
	f, err := s.requireProfile(id)
	if err != nil {
		return flow.Flow{}, err
	}
	if len(f.Nodes) > 0 {
		return flow.Flow{}, Errorf(KindInvalid, "profile %q already has nodes", id)
	}
	credential, err := s.seedCredential()
	if err != nil {
		return flow.Flow{}, err
	}
	if credential == "" {
		return flow.Flow{}, Errorf(KindInvalid, "Connect exactly one GitHub account to add the starter feeds.")
	}

	seed := starterSeed(credential)
	f.Nodes, f.Wires = seed.Nodes, seed.Wires
	if err := s.flows.Save(f); err != nil {
		return flow.Flow{}, Wrap(err, KindInvalid, "seeding profile %q", id)
	}
	if err := s.flows.SaveLayout(id, seed.Layout); err != nil {
		return flow.Flow{}, Wrap(err, KindInternal, "saving layout for profile %q", id)
	}
	s.notifyUpdated(ctx)

	seeded, ok := s.flows.Get(id)
	if !ok {
		return flow.Flow{}, Errorf(KindInternal, "profile %q vanished while seeding", id)
	}
	return seeded, nil
}

// Rename changes a flow's display name while preserving its stable id and
// graph definition.
func (s *FlowsService) Rename(ctx context.Context, id, name string) (flow.Flow, error) {
	if err := s.requireProfileExists(id); err != nil {
		return flow.Flow{}, err
	}
	f, err := s.flows.Rename(id, name)
	if err != nil {
		return flow.Flow{}, Wrap(err, KindInvalid, "renaming profile %q", id)
	}
	s.notifyUpdated(ctx)
	return f, nil
}

// SetEnabled controls whether a flow participates in polling and execution
// while preserving its feed data and graph definition.
func (s *FlowsService) SetEnabled(ctx context.Context, id string, enabled bool) (flow.Flow, error) {
	if err := s.requireProfileExists(id); err != nil {
		return flow.Flow{}, err
	}
	f, err := s.flows.SetEnabled(id, enabled)
	if err != nil {
		return flow.Flow{}, Wrap(err, KindInvalid, "updating profile %q", id)
	}
	s.notifyUpdated(ctx)
	return f, nil
}

// SetOrder persists the rail order as profiles.order and applies it to the
// live listing, so a reorder shows up without a restart. ids is the whole
// rail, top first — an id it leaves out falls back to sorting alphabetically
// behind the ones it names, which is also what a profile created later gets.
//
// Ids are not checked against the loaded flows. An id naming nothing is
// already ignored when the order is read, and refusing the write would make
// deleting a profile able to fail an unrelated reorder.
func (s *FlowsService) SetOrder(ctx context.Context, ids []string) error {
	if _, err := s.settings.Update(func(current *settings.Settings) error {
		current.Profiles.Order = ids
		return nil
	}); err != nil {
		return Wrap(err, KindInternal, "saving the profile order")
	}
	s.flows.SetOrder(ids)
	s.notifyUpdated(ctx)
	return nil
}

// Delete removes a profile's files before purging its durable state, in that
// order.
//
// Deleting something that is not there is KindNotFound rather than a silent
// success. This is the one irreversible operation on the surface, so a caller
// that mistyped an id has to learn that nothing was deleted — a constant
// success answer is indistinguishable from having destroyed the wrong profile.
//
// "Not there" means no flow file *and* no inbox rows, which is what keeps the
// files-first retry working: a deletion whose purge failed leaves the rows
// behind, and refusing the retry would strand them with no id left to name
// them by.
func (s *FlowsService) Delete(ctx context.Context, id string) error {
	if err := s.requireDeletable(ctx, id); err != nil {
		return err
	}
	if err := s.flows.Delete(id); err != nil {
		return Wrap(err, KindInvalid, "deleting profile %q", id)
	}
	// A leftover avatar is orphaned data, never a reason to fail the delete.
	_ = s.images.Delete(id)
	if err := s.EnsureProfile(ctx); err != nil {
		return Wrap(err, KindInternal, "replacing the last profile")
	}
	s.notifyUpdated(ctx)
	return Wrap(s.purgeProfile(ctx, id), KindInternal, "purging inbox rows for profile %q", id)
}

// purgeProfile removes cross-aggregate state in one transaction. Inbox events
// and feed claims cascade from the inbox-item delete.
func (s *FlowsService) purgeProfile(ctx context.Context, profileID string) error {
	topicPrefix := "source:" + profileID + "/"
	return s.stores.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.stores.InboxItems.DeleteByProfile(ctx, profileID); err != nil {
			return fmt.Errorf("purging inbox items: %w", err)
		}
		// The cascade above dropped every claim tied to a deleted item. This
		// catches a claim recorded under profileID against another profile's
		// item, which the cascade cannot reach.
		if err := s.stores.FeedClaims.DeleteByProfile(ctx, profileID); err != nil {
			return fmt.Errorf("purging feed membership claims: %w", err)
		}
		// The sessions themselves are hive's and survive; only the links go,
		// because there is no longer an item for them to hang off.
		if err := s.stores.ItemSessions.DeleteByProfile(ctx, profileID); err != nil {
			return fmt.Errorf("purging item session links: %w", err)
		}
		if err := s.stores.EventLog.DeleteByTopicPrefix(ctx, topicPrefix); err != nil {
			return fmt.Errorf("purging event log: %w", err)
		}
		if err := s.stores.EventLog.DeleteConsumerOffset(ctx, profileID); err != nil {
			return fmt.Errorf("purging consumer offset: %w", err)
		}
		if err := s.stores.SourceHeads.DeleteByTopicPrefix(ctx, topicPrefix); err != nil {
			return fmt.Errorf("purging source head: %w", err)
		}
		if err := s.stores.NodeKV.DeleteByFlow(ctx, profileID); err != nil {
			return fmt.Errorf("purging node kv: %w", err)
		}
		return nil
	})
}

// Get returns one flow's full definition for the editor.
func (s *FlowsService) Get(_ context.Context, id string) (flow.Flow, error) {
	return s.requireProfile(id)
}

// Save validates and persists a flow's definition. An invalid flow is
// rejected and the last-good file on disk — and the served flow — are left
// untouched.
func (s *FlowsService) Save(_ context.Context, f flow.Flow) error {
	return Wrap(s.flows.Save(f), KindInvalid, "saving profile %q", f.ID)
}

// SetProfileImage normalizes raw into the square avatar the rail draws, stores
// it under the data dir, and records its content hash on the flow. Replacing
// an existing image overwrites it.
func (s *FlowsService) SetProfileImage(ctx context.Context, id string, raw []byte) (flow.Flow, error) {
	if _, err := s.requireProfile(id); err != nil {
		return flow.Flow{}, err
	}
	hash, err := s.images.Set(id, raw)
	if err != nil {
		return flow.Flow{}, mapImageError(err)
	}
	f, err := s.flows.SetImage(id, hash)
	if err != nil {
		// The bytes landed but the reference did not; drop the orphan so a
		// retry starts clean and nothing renders an unreferenced file.
		_ = s.images.Delete(id)
		return flow.Flow{}, Wrap(err, KindInternal, "recording image for profile %q", id)
	}
	s.notifyUpdated(ctx)
	return f, nil
}

// ClearProfileImage removes a profile's avatar: the reference is dropped first
// so the rail stops drawing it even if the file removal that follows fails,
// leaving at worst an orphaned file the next set or delete reclaims.
func (s *FlowsService) ClearProfileImage(ctx context.Context, id string) (flow.Flow, error) {
	if _, err := s.requireProfile(id); err != nil {
		return flow.Flow{}, err
	}
	f, err := s.flows.SetImage(id, "")
	if err != nil {
		return flow.Flow{}, Wrap(err, KindInternal, "clearing image for profile %q", id)
	}
	_ = s.images.Delete(id)
	s.notifyUpdated(ctx)
	return f, nil
}

// ProfileImage returns a profile's stored avatar PNG, or nil when it has none.
func (s *FlowsService) ProfileImage(_ context.Context, id string) ([]byte, error) {
	data, ok, err := s.images.Get(id)
	if err != nil {
		return nil, Wrap(err, KindInternal, "reading image for profile %q", id)
	}
	if !ok {
		return nil, nil
	}
	return data, nil
}

// SetNodeImage normalizes raw into a feed-mark PNG, stores it, records its hash
// on source node nodeID in flow flowID, saves the flow, and returns the hash.
func (s *FlowsService) SetNodeImage(ctx context.Context, flowID, nodeID string, raw []byte) (string, error) {
	hash, err := s.marks.Set(raw)
	if err != nil {
		return "", mapMarkImageError(err)
	}
	if _, err := s.flows.SetSourceImage(flowID, nodeID, hash); err != nil {
		return "", mapNodeImageError(err, flowID, nodeID)
	}
	s.notifyUpdated(ctx)
	return hash, nil
}

// ClearNodeImage clears a source node's feed-mark image. The stored blob is left
// in place; it is content-addressed and may be shared.
func (s *FlowsService) ClearNodeImage(ctx context.Context, flowID, nodeID string) error {
	if _, err := s.flows.SetSourceImage(flowID, nodeID, ""); err != nil {
		return mapNodeImageError(err, flowID, nodeID)
	}
	s.notifyUpdated(ctx)
	return nil
}

// NodeImage returns a source node's stored feed-mark PNG, or nil when it has
// none or its hash resolves to no file.
func (s *FlowsService) NodeImage(_ context.Context, flowID, nodeID string) ([]byte, error) {
	hash, err := s.flows.SourceImageRef(flowID, nodeID)
	if err != nil {
		return nil, mapNodeImageError(err, flowID, nodeID)
	}
	if hash == "" {
		return nil, nil
	}
	data, ok, err := s.marks.Get(hash)
	if err != nil {
		return nil, Wrap(err, KindInternal, "reading mark image for node %q", nodeID)
	}
	if !ok {
		return nil, nil
	}
	return data, nil
}

// StoreMarkImage normalizes raw into a feed-mark PNG, stores it, and returns its
// content hash — the value a source node records in its `image` config. It does
// not touch the flow; the graph save records the hash.
func (s *FlowsService) StoreMarkImage(_ context.Context, raw []byte) (string, error) {
	hash, err := s.marks.Set(raw)
	if err != nil {
		return "", mapMarkImageError(err)
	}
	return hash, nil
}

// MarkImage returns the stored PNG for a mark hash, or ok=false when none is
// stored. A missing or malformed reference is not an error.
func (s *FlowsService) MarkImage(_ context.Context, hash string) (data []byte, ok bool, err error) {
	data, ok, err = s.marks.Get(hash)
	if err != nil {
		return nil, false, Wrap(err, KindInternal, "reading mark image %q", hash)
	}
	return data, ok, nil
}

// mapNodeImageError maps the flow store's node-image sentinels onto app kinds.
func mapNodeImageError(err error, flowID, nodeID string) error {
	switch {
	case errors.Is(err, flow.ErrFlowNotFound):
		return Errorf(KindNotFound, "profile %q not found", flowID)
	case errors.Is(err, flow.ErrNodeNotFound):
		return Errorf(KindNotFound, "node %q not found in profile %q", nodeID, flowID)
	case errors.Is(err, flow.ErrNodeNotImageMarkable):
		return Errorf(KindInvalid, "node %q does not support an image mark (only webhook and command sources do)", nodeID)
	default:
		return Wrap(err, KindInvalid, "setting image for node %q in profile %q", nodeID, flowID)
	}
}

// mapMarkImageError turns a normalization failure into a user-facing message.
func mapMarkImageError(err error) error {
	switch {
	case errors.Is(err, sourcemark.ErrEmpty):
		return Errorf(KindInvalid, "No image was provided.")
	case errors.Is(err, sourcemark.ErrUnsupported):
		return Errorf(KindInvalid, "That file isn't a supported image. Use PNG, JPEG, GIF, or WebP.")
	case errors.Is(err, sourcemark.ErrTooLarge):
		return Errorf(KindInvalid, "That image is too large. Choose a smaller file.")
	default:
		return Wrap(err, KindInternal, "processing image")
	}
}

// mapImageError turns a normalization failure into a user-facing message: the
// bad-input cases are KindInvalid so the settings view can show them verbatim,
// while an I/O failure stays internal.
func mapImageError(err error) error {
	switch {
	case errors.Is(err, profileimg.ErrEmpty):
		return Errorf(KindInvalid, "No image was provided.")
	case errors.Is(err, profileimg.ErrUnsupported):
		return Errorf(KindInvalid, "Unsupported image format. Use PNG, JPEG, GIF, or WebP.")
	case errors.Is(err, profileimg.ErrTooLarge):
		return Errorf(KindInvalid, "That image is too large. Choose a smaller one.")
	default:
		return Wrap(err, KindInternal, "processing image")
	}
}

// Layout returns a flow's canvas positions. A missing or broken layout file
// is not an error: an empty Layout tells the editor to lay out nodes fresh.
func (s *FlowsService) Layout(_ context.Context, id string) flow.Layout {
	return s.flows.GetLayout(id)
}

func (s *FlowsService) SaveLayout(_ context.Context, id string, layout flow.Layout) error {
	return Wrap(s.flows.SaveLayout(id, layout), KindInternal, "saving layout for profile %q", id)
}

// Sidebar returns how a flow's feed nodes are grouped into folders and
// ordered. A missing or broken file is not an error — an empty layout falls
// back to flow-node order.
func (s *FlowsService) Sidebar(_ context.Context, id string) flow.SidebarLayout {
	return s.flows.GetSidebar(id)
}

func (s *FlowsService) SaveSidebar(_ context.Context, id string, layout flow.SidebarLayout) error {
	return Wrap(s.flows.SaveSidebar(id, layout), KindInternal, "saving sidebar for profile %q", id)
}
