// Command release cuts one release of every program in this repository: it
// manages the release notes, publishes Hive Desktop, tags the shared version,
// and dispatches the workflow that ships the hive CLI.
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

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/releasenotes"
)

func main() {
	logger := newReleaseLogger(os.Stderr)
	ctx := logger.WithContext(context.Background())
	if err := newReleaseCommand().Run(ctx, os.Args); err != nil {
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
		Description: "Selects and validates the shared version against Git tags, live desktop manifests, and the promoted release notes, " +
			"then builds, signs, notarizes, uploads, and verifies the desktop, and dispatches the CLI's publish workflow.",
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "interactively prepare and publish a release",
				Description: "Refreshes and validates the release source, takes the version from the newest promoted release notes, presents it for explicit confirmation, " +
					"runs the release gates, revalidates the confirmed commit, then publishes. Run through `mise run release` so mise loads credentials.",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "dry-run", Usage: "exercise candidate selection and confirmation without gates, builds, uploads, tags, or a workflow dispatch"},
				},
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 0 {
						return cli.Exit("expected no arguments: the version comes from the promoted release notes", 2)
					}
					return runInteractiveRelease(ctx, cmd.Bool("dry-run"))
				}),
			},
			{
				Name:  "next",
				Usage: "print the version a release cut now would take",
				Description: "Prints 0.YYYYMMDD.N for today's UTC date, where N counts the releases already published that day. It reads every v* and desktop-v* tag " +
					"and the live desktop manifests. A missing live manifest (HTTP 404) is an empty channel.",
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 0 {
						return cli.Exit("expected no arguments", 2)
					}
					versions, _, err := releaseVersions(ctx)
					if err != nil {
						return err
					}
					version, err := nextVersion(time.Now(), versions)
					if err != nil {
						return err
					}
					fmt.Println(version)
					return nil
				}),
			},
			{
				Name:      "prepare",
				Usage:     "select and validate a release candidate",
				ArgsUsage: "[version]",
				Description: "Requires a clean current main, takes the version from the newest promoted release notes when version is omitted, validates that it advances " +
					"every tag and live manifest, requires an entry with a summary for every program, rejects an existing local or origin tag, and prints the plan.",
				Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() > 1 {
						return cli.Exit("expected an optional version", 2)
					}
					return prepare(ctx, cmd.Args().First())
				}),
			},
			{
				Name:      "publish",
				Usage:     "build, sign, notarize, and publish a release",
				ArgsUsage: "<version>",
				Description: "Public publishing requires a clean current main or a matching release tag on main; the command builds the universal macOS app, signs it, " +
					"notarizes and staples it, packages and verifies it, uploads immutable artifacts to R2, writes every desktop manifest, verifies the public artifact, " +
					"then tags the release and dispatches the CLI's publish workflow. Run this through `mise run release:publish -- <version>` so mise loads credentials.",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "skip-notarize", Usage: "skip notarization and stapling (requires --skip-upload)"},
					&cli.BoolFlag{Name: "skip-upload", Usage: "build and package without publishing"},
					&cli.BoolFlag{Name: "force", Usage: "permit overwriting an existing immutable release"},
					&cli.BoolFlag{Name: "resume", Usage: "reuse verified cmd/desktop/bin artifacts and finish an interrupted upload without rebuilding"},
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
				Usage:     "push the release tag and dispatch the publish workflow",
				ArgsUsage: "<version>",
				Description: "Pushes the lightweight v<version> tag and dispatches publish.yml, which builds the CLI with GoReleaser, creates the GitHub release " +
					"with every program's notes, and deploys the site. It waits for the run. Desktop downloads still come from R2 (decision 0003). " +
					"Idempotent: safe to re-run for a release whose GitHub step failed after the R2 upload. Requires an authenticated gh.",
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
				Usage: "manage each program's release notes",
				Commands: []*cli.Command{
					{
						Name:      "promote",
						Usage:     "turn every program's accumulated draft into its changelog entry for the next release",
						ArgsUsage: "[version]",
						Description: "Collapses each program's changelog/unreleased/ into its <version>.md, stamping the version and date, and " +
							"deletes the fragments. The version defaults to `release next`, and it is the version the release publishes. Every program " +
							"gets an entry, because every release ships every program. Edit the entries before committing them: each is the sum of " +
							"every pull request since the last release, so consolidate near-duplicate bullets and write the summaries. Commit the " +
							"result before releasing: the release ships the notes main holds, and `release publish` refuses a version with no entry.",
						Action: withRepoRoot(func(ctx context.Context, cmd *cli.Command) error {
							if cmd.NArg() > 1 {
								return cli.Exit("expected an optional version", 2)
							}
							version, err := promoteTargetVersion(ctx, cmd.Args().First())
							if err != nil {
								return err
							}
							paths, err := promoteDrafts(ctx, version)
							if err != nil {
								return err
							}
							for _, path := range paths {
								fmt.Printf("wrote %s\n", path)
							}
							fmt.Println("consolidate the bullets and write each summary, then run `mise run changelog:pr`")
							return nil
						}),
					},
					{
						Name:  "pr",
						Usage: "open the pull request that lands the promoted release notes",
						Description: "Commits the entries that `changelog promote` wrote, plus the fragments it deleted, on a branch of its own, " +
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
						Name:      "notes",
						Usage:     "print the GitHub release body for a version",
						ArgsUsage: "<version>",
						Description: "Prints the desktop download header and every program's entry for the version, each under its own heading. " +
							"The publish workflow passes it to GoReleaser as the release notes.",
						Action: withRepoRoot(func(_ context.Context, cmd *cli.Command) error {
							if cmd.NArg() != 1 {
								return cli.Exit("expected exactly one version", 2)
							}
							version, err := parsePublishVersion(cmd.Args().First())
							if err != nil {
								return fmt.Errorf("invalid version %q: %w", cmd.Args().First(), err)
							}
							body, err := releaseNotesBody(version, downloadBaseURL())
							if err != nil {
								return err
							}
							fmt.Print(body)
							return nil
						}),
					},
					{
						Name:      "new",
						Usage:     "write one unreleased change to the changelog draft",
						ArgsUsage: "<note>",
						Description: "Adds a file to the program's changelog/unreleased/ holding one bullet of its release notes. " +
							"The name is built from a UTC timestamp and the note, so concurrent branches each add a file instead of " +
							"conflicting over one. The note is product copy a user reads — read the release-notes skill " +
							"before writing one. Pass it as the argument, or on stdin for a note that spans lines.",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "product",
								Aliases:  []string{"p"},
								Usage:    fmt.Sprintf("the program the change ships in: one of %v", productNames()),
								Required: true,
							},
							&cli.StringFlag{
								Name:     "kind",
								Aliases:  []string{"k"},
								Usage:    "the section it belongs under: added, changed, or fixed",
								Required: true,
							},
						},
						Action: withRepoRoot(func(_ context.Context, cmd *cli.Command) error {
							product, err := parseProduct(cmd.String("product"))
							if err != nil {
								return cli.Exit(err.Error(), 2)
							}
							kind, ok := releasenotes.ParseKind(cmd.String("kind"))
							if !ok {
								return cli.Exit(fmt.Sprintf("--kind must be one of %v", releasenotes.Kinds), 2)
							}
							note, err := fragmentNote(cmd)
							if err != nil {
								return err
							}
							path, err := newFragment(product, kind, note)
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
	commit           string
	subject          string
	currentManifests map[string]string
}

func prepare(ctx context.Context, candidate string) error {
	plan, err := planRelease(ctx, candidate, true)
	if err != nil {
		return err
	}
	printReleasePlan(plan)
	return nil
}

// planRelease selects and validates the version to publish. With no
// candidate it is the pending version: the promoted release notes name it.
func planRelease(ctx context.Context, candidate string, validateSource bool) (releasePlan, error) {
	if validateSource {
		if err := validatePrepareSource(ctx); err != nil {
			return releasePlan{}, err
		}
	}
	if err := verifyMigrationOrder(ctx); err != nil {
		return releasePlan{}, err
	}
	published, manifests, err := releaseVersions(ctx)
	if err != nil {
		return releasePlan{}, err
	}
	var version releaseVersion
	if candidate == "" {
		version, err = pendingVersion(published)
	} else {
		version, err = parsePublishVersion(candidate)
	}
	if err != nil {
		return releasePlan{}, err
	}
	if newest, ok := newestVersion(published); ok && compareVersions(version, newest) <= 0 {
		return releasePlan{}, fmt.Errorf("candidate %s does not advance the newest published version %s", version, newest)
	}
	if err := validateManifestAdvancement(version, manifests); err != nil {
		return releasePlan{}, err
	}
	if err := validateChangelogEntry(version); err != nil {
		return releasePlan{}, err
	}

	tag := version.tag()
	if exists, err := localTagExists(ctx, tag); err != nil {
		return releasePlan{}, err
	} else if exists {
		return releasePlan{}, fmt.Errorf("tag %s already exists locally", tag)
	}
	if sha, err := originTagCommit(ctx, tag); err != nil {
		return releasePlan{}, err
	} else if sha != "" {
		return releasePlan{}, fmt.Errorf("tag %s already exists on origin", tag)
	}

	commit, err := commandOutput(ctx, "git", "show", "-s", "--format=%H%n%s", "HEAD")
	if err != nil {
		return releasePlan{}, err
	}
	lines := strings.SplitN(strings.TrimSpace(commit), "\n", 2)
	plan := releasePlan{
		version:          version,
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
	fmt.Printf("tag: %s\n", plan.version.tag())
	fmt.Printf("commit: %s\n", plan.commit)
	if plan.subject != "" {
		fmt.Printf("subject: %s\n", plan.subject)
	}
	fmt.Printf("manifests: %s\n", strings.Join(manifestChannels, "+"))
	for _, channel := range manifestChannels {
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
