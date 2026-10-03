package app

import (
	"context"
	"errors"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf"
	"github.com/colonyops/hive/internal/config"
)

// HiveSetup is the external Hive configuration as the first-run and settings
// screens see it: what the file declares, what this machine could run, and
// whether the environment is overriding the choice.
type HiveSetup struct {
	Config hiveconf.Setup `json:"config"`
	// Agents is the catalog marked by what resolved on the launch PATH.
	Agents []hiveconf.AgentOption `json:"agents"`
	// DefaultAgentOverride is HIVE_DEFAULT_AGENT when it is set and names a
	// profile the file declares — that is, when it would actually win over
	// agents.default at load. It is reported rather than hidden because
	// without it, a user who exports the variable picks an agent in the UI,
	// sees it saved, and watches sessions start with a different one.
	DefaultAgentOverride string `json:"defaultAgentOverride"`
}

// HiveSetupRequest is a whole edit, not a delta: the screen holds every
// profile and every workspace while it is open, so a save reconciles the file
// to exactly this set. Profiles the screen loaded and did not touch come back
// unchanged, which is what keeps a hand-written profile from being dropped by
// an edit that was only about workspaces.
type HiveSetupRequest struct {
	DefaultAgent string             `json:"defaultAgent"`
	Profiles     []hiveconf.Profile `json:"profiles"`
	Workspaces   []string           `json:"workspaces"`
}

type hiveConfigOptions struct {
	Location func() HiveConfigLocation
	LookPath hiveconf.LookPath
	// DefaultAgent reads HIVE_DEFAULT_AGENT the way the user's terminal would.
	// nil means NopDefaultAgentReader.
	DefaultAgent DefaultAgentReader
	// Check is hive's own loader, run against a written candidate before it
	// replaces the file. config.ApplyEdit validates the two keys it owns; only hive
	// can say whether the whole file still loads.
	Check config.Check
	// Reload rebuilds the Hive-config-derived services. Save calls it so a
	// write takes effect in the running process instead of at the next launch
	// (ADR the-hive-runtime-rebinds-on-a-config-write-instead-of-requiring-a-restart).
	Reload func(context.Context) error
}

// HiveConfigService reads and writes the Hive CLI configuration the desktop
// shares with the `hive` binary.
//
// It is separate from SystemService, which owns the app's own locations: this
// one owns a file another product also writes, and the whole of its contract
// is that an edit touches the two keys the desktop asked about and nothing
// else.
type HiveConfigService struct {
	location     func() HiveConfigLocation
	lookPath     hiveconf.LookPath
	defaultAgent DefaultAgentReader
	check        config.Check
	reload       func(context.Context) error
}

func newHiveConfigService(opts hiveConfigOptions) *HiveConfigService {
	if opts.DefaultAgent == nil {
		opts.DefaultAgent = NopDefaultAgentReader{}
	}
	return &HiveConfigService{
		location:     opts.Location,
		lookPath:     opts.LookPath,
		defaultAgent: opts.DefaultAgent,
		check:        opts.Check,
		reload:       opts.Reload,
	}
}

// Setup reports the current configuration and the agents this machine can run.
func (s *HiveConfigService) Setup(ctx context.Context) HiveSetup {
	location := s.location()
	setup := hiveconf.Load(location.Path)
	names := make([]string, 0, len(setup.Profiles))
	for _, profile := range setup.Profiles {
		names = append(names, profile.Name)
	}
	return HiveSetup{
		Config:               setup,
		Agents:               hiveconf.AgentOptions(ctx, s.lookPath),
		DefaultAgentOverride: environmentAgentOverride(s.defaultAgent.DefaultAgent(ctx), names),
	}
}

// InspectWorkspace validates a candidate parent folder and reports its
// immediate repository count.
func (s *HiveConfigService) InspectWorkspace(_ context.Context, path string) (hiveconf.Workspace, error) {
	if err := config.CheckWorkspace(path); err != nil {
		return hiveconf.Workspace{}, Wrap(err, KindInvalid, "%s", path)
	}
	return hiveconf.Inspect(path), nil
}

// Save writes the edit and reloads the runtime from it.
//
// The write is validated first, checked against hive's loader, and atomic, so
// a rejected edit leaves the file exactly as it was. A reload failure after
// that is reported, not swallowed: the file on disk loads, so the honest
// answer is that it needs a restart to take effect.
func (s *HiveConfigService) Save(ctx context.Context, req HiveSetupRequest) (HiveSetup, error) {
	edit := config.Edit{
		DefaultAgent: req.DefaultAgent,
		Profiles:     make([]config.EditProfile, 0, len(req.Profiles)),
		Workspaces:   req.Workspaces,
	}
	for _, p := range req.Profiles {
		edit.Profiles = append(edit.Profiles, config.EditProfile{Name: p.Name, Command: p.Command, Flags: p.Flags})
	}
	if err := config.ApplyEdit(s.location().Path, edit, s.check); err != nil {
		return HiveSetup{}, Wrap(err, hiveWriteKind(err), "saving the Hive configuration")
	}
	if s.reload != nil {
		if err := s.reload(ctx); err != nil {
			return HiveSetup{}, Wrap(err, KindInternal, "the Hive configuration was saved, but reloading it failed — restart Hive to apply it")
		}
	}
	return s.Setup(ctx), nil
}

// hiveWriteKind separates an edit the user can fix from a disk that would not
// take the write. Only the former is worth showing beside the field that
// caused it.
func hiveWriteKind(err error) Kind {
	if _, ok := errors.AsType[config.InvalidEditError](err); ok {
		return KindInvalid
	}
	return KindInternal
}
