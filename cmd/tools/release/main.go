// Command release holds the release steps of the programs in this repository:
// the Hive Desktop publisher and changelog commands, and the hive CLI's tag
// selection under `release cli`.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/releasenotes"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/tools/release/internal/commands"
)

func main() {
	logger := newReleaseLogger(os.Stderr)
	ctx := logger.WithContext(context.Background())
	command := commands.NewCLICmd().Register(newReleaseCommand())
	if err := command.Run(ctx, os.Args); err != nil {
		logger.Error().Err(err).Msg("release failed")
		os.Exit(1)
	}
}

func newReleaseLogger(out io.Writer) zerolog.Logger {
	writer := zerolog.ConsoleWriter{Out: out, TimeFormat: time.Kitchen}
	return zerolog.New(writer).With().Timestamp().Logger()
}

func newReleaseCommand() *cli.Command {
	return &cli.Command{
		Name:  "release",
		Usage: "release steps for the programs in this repository",
		Description: "Selects and validates versions against both Git tags and live channel manifests, " +
			"then builds, signs, notarizes, uploads, and verifies desktop releases.",
		Commands: []*cli.Command{
			{
				Name:      "run",
				Usage:     "interactively prepare and publish a release",
				ArgsUsage: "[dev|beta|stable] [version]",
				Description: "Selects a channel and patch, minor, or major increment when omitted, refreshes and validates the release source, presents the candidate for explicit confirmation, " +
					"runs the release gates, revalidates the confirmed commit, then publishes. Run through `mise run desktop:release` so mise loads credentials.",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "dry-run", Usage: "exercise candidate selection and confirmation without gates, builds, uploads, tags, or a GitHub release"},
				},
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					channel, candidate, err := parseInteractiveReleaseArgs(cmd.Args().Slice())
					if err != nil {
						return cli.Exit(err.Error(), 2)
					}
					return runInteractiveRelease(ctx, channel, candidate, cmd.Bool("dry-run"))
				}),
			},
			{
				Name:      "next",
				Usage:     "print the next version for a release channel",
				ArgsUsage: "<dev|beta|stable>",
				Description: "Selects the next version from all local desktop-v* tags and the live stable, " +
					"beta, and dev manifests. A missing live manifest (HTTP 404) is an empty channel.",
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 || !validChannel(cmd.Args().First()) {
						return cli.Exit("expected exactly one channel: dev, beta, or stable", 2)
					}
					versions, _, err := releaseVersions(ctx)
					if err != nil {
						return err
					}
					fmt.Println(nextVersion(cmd.Args().First(), versions))
					return nil
				}),
			},
			{
				Name:      "prepare",
				Usage:     "select and validate a release candidate",
				ArgsUsage: "<dev|beta|stable> [version]",
				Description: "Requires a clean current main, selects the next version when version is omitted, validates advancement " +
					"across every affected live manifest, rejects existing local or origin tags, and prints the commit and manifest cascade.",
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() < 1 || cmd.NArg() > 2 || !validChannel(cmd.Args().First()) {
						return cli.Exit("expected a channel (dev, beta, or stable) and optional version", 2)
					}
					return prepare(ctx, cmd.Args().First(), cmd.Args().Get(1))
				}),
			},
			{
				Name:      "publish",
				Usage:     "build, sign, notarize, and publish a release",
				ArgsUsage: "<version>",
				Description: "Public publishing requires a clean current main or a matching CI tag on main; the command builds the universal macOS app, signs it, " +
					"notarizes and staples it, packages and verifies it, uploads immutable artifacts to R2, updates the channel cascade, and verifies the public artifact. " +
					"For local publishing, run this through `mise run release:publish -- <version>` so mise loads credentials.",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "skip-notarize", Usage: "skip notarization and stapling (requires --skip-upload)"},
					&cli.BoolFlag{Name: "skip-upload", Usage: "build and package without publishing"},
					&cli.BoolFlag{Name: "force", Usage: "permit overwriting an existing immutable release"},
					&cli.BoolFlag{Name: "resume", Usage: "reuse verified desktop/bin artifacts and finish an interrupted upload without rebuilding"},
				},
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 {
						return cli.Exit("expected exactly one version", 2)
					}
					args := []string{cmd.Args().First()}
					for _, flag := range []string{"skip-notarize", "skip-upload", "force", "resume"} {
						if cmd.Bool(flag) {
							args = append(args, "--"+flag)
						}
					}
					return publish(ctx, args)
				}),
			},
			{
				Name:      "github",
				Usage:     "push the release tag and create the GitHub release",
				ArgsUsage: "<version>",
				Description: "Records a published version on GitHub: pushes the lightweight desktop-v<version> tag and creates a GitHub Release whose body is " +
					"the version's committed changelog entry (dev and beta are marked prerelease). Downloads still come from R2 (decision 0003); " +
					"this attaches no artifacts. Idempotent — safe to re-run to record a release whose GitHub step failed after the R2 upload. Requires an authenticated gh.",
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 {
						return cli.Exit("expected exactly one version", 2)
					}
					version, err := parsePublishVersion(cmd.Args().First())
					if err != nil {
						return fmt.Errorf("invalid version %q: %w", cmd.Args().First(), err)
					}
					if err := validatePublishSource(ctx, version); err != nil {
						return err
					}
					return publishGitHubRelease(ctx, version)
				}),
			},
			{
				Name:  "changelog",
				Usage: "manage the release notes embedded in the app",
				Commands: []*cli.Command{
					{
						Name:      "promote",
						Usage:     "turn the accumulated draft into a stable release's changelog entry",
						ArgsUsage: "<stable|version>",
						Description: "Collapses cmd/desktop/releasenotes/changelog/unreleased/ into <version>.md, stamping the version and date, and " +
							"deletes the fragments. Edit the entry before committing it: it is the sum of every pull request since the last " +
							"release, so consolidate near-duplicate bullets and write the summary. Commit the result before releasing: the notes " +
							"are embedded in the binary, and `release publish` refuses a stable version that has no entry. Prereleases need none " +
							"— they publish the draft as it stands.",
						Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
							if cmd.NArg() != 1 {
								return cli.Exit("expected \"stable\" or an explicit stable version", 2)
							}
							version, err := promoteTargetVersion(ctx, cmd.Args().First())
							if err != nil {
								return err
							}
							path, err := promoteDraft(ctx, version)
							if err != nil {
								return err
							}
							fmt.Printf("wrote %s\n", path)
							fmt.Println("consolidate the bullets and write its summary, then run `mise run changelog:pr`")
							return nil
						}),
					},
					{
						Name:  "pr",
						Usage: "open the pull request that lands the promoted release notes",
						Description: "Commits the entry that `changelog promote` wrote, plus the fragments it deleted, on a branch of its own, " +
							"and opens its pull request. It refuses a worktree that holds anything else, and an entry whose summary is " +
							"still empty. Run it after you edit the entry. The release itself cannot write the entry, because a release " +
							"requires a clean tree identical to origin/main.",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "print the branch, commit and pull request instead of creating them",
							},
						},
						Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
							return openReleaseNotesPR(ctx, cmd.Bool("dry-run"))
						}),
					},
					{
						Name:      "new",
						Usage:     "write one unreleased change to the changelog draft",
						ArgsUsage: "<note>",
						Description: "Adds a file to cmd/desktop/releasenotes/changelog/unreleased/ holding one bullet of the release notes. " +
							"The name is built from a UTC timestamp and the note, so concurrent branches each add a file instead of " +
							"conflicting over one. The note is product copy a user reads inside the app — read the release-notes skill " +
							"before writing one. Pass it as the argument, or on stdin for a note that spans lines.",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "kind",
								Aliases:  []string{"k"},
								Usage:    "the section it belongs under: added, changed, or fixed",
								Required: true,
							},
						},
						Action: withRepoRoot(func(_ context.Context, cmd *cli.Command) error {
							kind, ok := releasenotes.ParseKind(cmd.String("kind"))
							if !ok {
								return cli.Exit(fmt.Sprintf("--kind must be one of %v", releasenotes.Kinds), 2)
							}
							note, err := fragmentNote(cmd)
							if err != nil {
								return err
							}
							path, err := newFragment(kind, note)
							if err != nil {
								return err
							}
							fmt.Println(path)
							return nil
						}),
					},
				},
			},
			{
				Name:      "verify",
				Usage:     "verify live manifests and the public artifact",
				ArgsUsage: "<version>",
				Description: "Requires every affected manifest to agree on the version and artifact metadata, " +
					"then downloads the artifact and verifies its size and SHA-256 checksum.",
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 {
						return cli.Exit("expected exactly one version", 2)
					}
					version, err := parsePublishVersion(cmd.Args().First())
					if err != nil {
						return fmt.Errorf("invalid version %q: %w", cmd.Args().First(), err)
					}
					return verifyLive(ctx, version)
				}),
			},
		},
	}
}

func withRepoRoot(action cli.ActionFunc) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		root, err := commandOutput(ctx, "git", "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
		if err := os.Chdir(strings.TrimSpace(root)); err != nil {
			return fmt.Errorf("change to repository root: %w", err)
		}
		return action(ctx, cmd)
	}
}

type releasePlan struct {
	version          releaseVersion
	channel          string
	commit           string
	subject          string
	currentManifests map[string]string
}

func prepare(ctx context.Context, channel, candidate string) error {
	plan, err := prepareReleasePlan(ctx, channel, candidate)
	if err != nil {
		return err
	}
	printReleasePlan(plan)
	return nil
}

func prepareReleasePlan(ctx context.Context, channel, candidate string) (releasePlan, error) {
	return planRelease(ctx, channel, candidate, true)
}

func previewReleasePlan(ctx context.Context, channel, candidate string) (releasePlan, error) {
	return planRelease(ctx, channel, candidate, false)
}

func planRelease(ctx context.Context, channel, candidate string, validateSource bool) (releasePlan, error) {
	if validateSource {
		if err := validatePrepareSource(ctx); err != nil {
			return releasePlan{}, err
		}
	}
	if err := verifyMigrationOrder(ctx); err != nil {
		return releasePlan{}, err
	}
	versions, manifests, err := releaseVersions(ctx)
	if err != nil {
		return releasePlan{}, err
	}
	if candidate == "" {
		candidate = nextVersion(channel, versions)
	}
	version, err := parsePublishVersion(candidate)
	if err != nil {
		return releasePlan{}, fmt.Errorf("invalid candidate %q: %w", candidate, err)
	}
	if version.channel() != channel {
		return releasePlan{}, fmt.Errorf("candidate %s belongs to %s, not %s", candidate, version.channel(), channel)
	}
	if err := validateManifestAdvancement(version, manifests); err != nil {
		return releasePlan{}, err
	}
	if err := validateChangelogEntry(version); err != nil {
		return releasePlan{}, err
	}

	tag := "desktop-v" + version.String()
	if exists, err := localTagExists(ctx, tag); err != nil {
		return releasePlan{}, err
	} else if exists {
		return releasePlan{}, fmt.Errorf("tag %s already exists locally", tag)
	}
	remote, err := commandOutput(ctx, "git", "ls-remote", "--tags", "origin", "refs/tags/"+tag)
	if err != nil {
		return releasePlan{}, fmt.Errorf("check origin tag %s: %w", tag, err)
	}
	if strings.TrimSpace(remote) != "" {
		return releasePlan{}, fmt.Errorf("tag %s already exists on origin", tag)
	}

	commit, err := commandOutput(ctx, "git", "show", "-s", "--format=%H%n%s", "HEAD")
	if err != nil {
		return releasePlan{}, err
	}
	lines := strings.SplitN(strings.TrimSpace(commit), "\n", 2)
	plan := releasePlan{
		version:          version,
		channel:          channel,
		commit:           lines[0],
		currentManifests: make(map[string]string, len(manifests)),
	}
	if len(lines) == 2 {
		plan.subject = lines[1]
	}
	for manifestChannel, manifest := range manifests {
		plan.currentManifests[manifestChannel] = manifest.Version
	}
	return plan, nil
}

func printReleasePlan(plan releasePlan) {
	fmt.Printf("candidate: %s\n", plan.version.String())
	fmt.Printf("channel: %s\n", plan.channel)
	fmt.Printf("commit: %s\n", plan.commit)
	if plan.subject != "" {
		fmt.Printf("subject: %s\n", plan.subject)
	}
	fmt.Printf("manifests: %s\n", strings.Join(plan.version.affectedChannels(), "+"))
	for _, channel := range []string{"stable", "beta", "dev"} {
		if version, ok := plan.currentManifests[channel]; ok {
			fmt.Printf("current-%s: %s\n", channel, version)
		} else {
			fmt.Printf("current-%s: empty\n", channel)
		}
	}
}

func verifyMigrationOrder(ctx context.Context) error {
	if err := quietCommand(ctx, "./cmd/desktop/scripts/check-migration-order.sh"); err != nil {
		return fmt.Errorf("verify migration order: %w", err)
	}
	return nil
}

func localTagExists(ctx context.Context, tag string) (bool, error) {
	output, err := commandOutput(ctx, "git", "tag", "--list", tag)
	if err != nil {
		return false, fmt.Errorf("check local tag %s: %w", tag, err)
	}
	return strings.TrimSpace(output) != "", nil
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.Output()
	if err != nil {
		exitErr := new(exec.ExitError)
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("%s: %s", name, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(output), nil
}

// fragmentNote reads the release note from the arguments, or from stdin when
// none are given, so a note that spans lines does not have to survive shell
// quoting.
func fragmentNote(cmd *cli.Command) (string, error) {
	if cmd.NArg() > 0 {
		return strings.Join(cmd.Args().Slice(), " "), nil
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read note from stdin: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return "", cli.Exit("expected the note as an argument or on stdin", 2)
	}
	return string(raw), nil
}
