package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

// cliTagPattern selects the release tags of the CLI. The repository also holds
// tags of other programs, such as desktop/v1.2.3, and `git describe` without a
// pattern returns the nearest tag of any name.
const cliTagPattern = "v[0-9]*"

var errNoCLITag = errors.New("no v* tag reachable from HEAD")

func newCLICommand() *cli.Command {
	return &cli.Command{
		Name:  "cli",
		Usage: "release steps for the hive CLI",
		Commands: []*cli.Command{
			{
				Name:      "tag",
				Usage:     "select the release tag for HEAD, create it, and push it",
				ArgsUsage: "<patch|minor|major>",
				Description: "Finds the newest v* tag on the ancestry of HEAD, bumps it, tags HEAD, and pushes the tag. " +
					"A v* tag that is already on HEAD is used again and the bump level is ignored, so a release that " +
					"failed after the tag push can run again on the same commit. Prints current=<tag> previous=<tag>, " +
					"and writes the two values to the GITHUB_OUTPUT file when that variable is set.",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "dry-run", Usage: "create the tag in the local repository only and push nothing"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 {
						return errors.New("expected one argument: patch, minor, or major")
					}
					level, err := parseCLIBumpLevel(cmd.Args().First())
					if err != nil {
						return err
					}
					return runCLITag(ctx, level, cmd.Bool("dry-run"), os.Stdout, os.Stderr)
				},
			},
		},
	}
}

type cliBumpLevel string

const (
	cliBumpPatch cliBumpLevel = "patch"
	cliBumpMinor cliBumpLevel = "minor"
	cliBumpMajor cliBumpLevel = "major"
)

func parseCLIBumpLevel(value string) (cliBumpLevel, error) {
	switch level := cliBumpLevel(value); level {
	case cliBumpPatch, cliBumpMinor, cliBumpMajor:
		return level, nil
	default:
		return "", fmt.Errorf("unknown bump level %q: expected patch, minor, or major", value)
	}
}

type cliVersion struct {
	major int
	minor int
	patch int
}

// parseCLIVersion reads vMAJOR.MINOR.PATCH. It drops a prerelease or build
// suffix, because a bump from v1.2.3-rc1 starts at 1.2.3.
func parseCLIVersion(tag string) (cliVersion, error) {
	value, ok := strings.CutPrefix(tag, "v")
	if !ok {
		return cliVersion{}, fmt.Errorf("tag %q does not start with v", tag)
	}
	if i := strings.IndexAny(value, "-+"); i >= 0 {
		value = value[:i]
	}

	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return cliVersion{}, fmt.Errorf("tag %q is not vMAJOR.MINOR.PATCH", tag)
	}

	numbers := make([]int, len(parts))
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return cliVersion{}, fmt.Errorf("tag %q is not vMAJOR.MINOR.PATCH", tag)
		}
		numbers[i] = number
	}

	return cliVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}, nil
}

func (v cliVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

func (v cliVersion) bump(level cliBumpLevel) cliVersion {
	switch level {
	case cliBumpMajor:
		return cliVersion{major: v.major + 1}
	case cliBumpMinor:
		return cliVersion{major: v.major, minor: v.minor + 1}
	default:
		return cliVersion{major: v.major, minor: v.minor, patch: v.patch + 1}
	}
}

func compareCLIVersion(left, right cliVersion) int {
	return cmp.Or(
		cmp.Compare(left.major, right.major),
		cmp.Compare(left.minor, right.minor),
		cmp.Compare(left.patch, right.patch),
	)
}

// cliTagState is what the repository says about the CLI tags near HEAD.
type cliTagState struct {
	head string
	// headTags are the CLI tags on HEAD.
	headTags []string
	// nearest is the newest CLI tag on the ancestry of HEAD, HEAD included.
	nearest string
	// nearestParent is the same for the first parent of HEAD.
	nearestParent string
}

type cliTagPlan struct {
	current  string
	previous string
	// reuse is true when HEAD already has the tag.
	reuse bool
}

func planCLITag(state cliTagState, level cliBumpLevel) (cliTagPlan, error) {
	if current, ok := newestCLITag(state.headTags); ok {
		return cliTagPlan{current: current, previous: state.nearestParent, reuse: true}, nil
	}

	if state.nearest == "" {
		return cliTagPlan{}, errNoCLITag
	}
	previous, err := parseCLIVersion(state.nearest)
	if err != nil {
		return cliTagPlan{}, err
	}

	return cliTagPlan{current: previous.bump(level).String(), previous: state.nearest}, nil
}

// newestCLITag ignores a tag that is not a version. The tag pattern is a glob,
// so it also matches a name such as v2-experiment.
func newestCLITag(tags []string) (string, bool) {
	var (
		newest    string
		newestVer cliVersion
	)
	for _, tag := range tags {
		version, err := parseCLIVersion(tag)
		if err != nil {
			continue
		}
		if newest == "" || compareCLIVersion(version, newestVer) > 0 {
			newest, newestVer = tag, version
		}
	}
	return newest, newest != ""
}

func readCLITagState(ctx context.Context) (cliTagState, error) {
	head, err := gitHead(ctx)
	if err != nil {
		return cliTagState{}, err
	}

	output, err := commandOutput(ctx, "git", "tag", "--points-at", "HEAD", "--list", cliTagPattern)
	if err != nil {
		return cliTagState{}, fmt.Errorf("list tags on HEAD: %w", err)
	}

	nearest, err := describeCLITag(ctx, "HEAD")
	if err != nil {
		return cliTagState{}, err
	}
	nearestParent, err := describeCLITag(ctx, "HEAD^")
	if err != nil {
		return cliTagState{}, err
	}

	return cliTagState{
		head:          head,
		headTags:      strings.Fields(output),
		nearest:       nearest,
		nearestParent: nearestParent,
	}, nil
}

// describeCLITag returns "" when the revision has no CLI tag on its ancestry,
// and when the revision does not exist, which is the case for the parent of
// the first commit.
func describeCLITag(ctx context.Context, revision string) (string, error) {
	output, err := commandOutput(ctx, "git", "describe", "--tags", "--abbrev=0", "--match", cliTagPattern, revision)
	if err == nil {
		return strings.TrimSpace(output), nil
	}

	for _, none := range []string{"No names found", "No tags can describe", "Not a valid object name"} {
		if strings.Contains(err.Error(), none) {
			return "", nil
		}
	}
	return "", fmt.Errorf("find the CLI tag before %s: %w", revision, err)
}

func runCLITag(ctx context.Context, level cliBumpLevel, dryRun bool, stdout, stderr io.Writer) error {
	state, err := readCLITagState(ctx)
	if err != nil {
		return err
	}
	plan, err := planCLITag(state, level)
	if err != nil {
		return err
	}

	if plan.reuse {
		_, _ = fmt.Fprintf(stderr, "release: HEAD already tagged %s; reusing it, bump level ignored\n", plan.current)
	} else {
		// The local tag is necessary in a dry run too: GoReleaser reads it.
		if err := runCommand(ctx, "git", "tag", "-a", plan.current, "-m", plan.current, "HEAD"); err != nil {
			return err
		}
		if !dryRun {
			if err := runCommand(ctx, "git", "push", "origin", "refs/tags/"+plan.current); err != nil {
				return err
			}
		}
	}

	at, err := commandOutput(ctx, "git", "rev-list", "-n", "1", plan.current)
	if err != nil {
		return fmt.Errorf("resolve tag %s: %w", plan.current, err)
	}
	if commit := strings.TrimSpace(at); commit != state.head {
		return fmt.Errorf("tag %s points at %s, not at HEAD %s", plan.current, commit, state.head)
	}

	values := fmt.Sprintf("current=%s\nprevious=%s\n", plan.current, plan.previous)
	_, _ = fmt.Fprintf(stdout, "current=%s previous=%s\n", plan.current, plan.previous)
	return writeGitHubOutput(values)
}

func writeGitHubOutput(values string) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open GITHUB_OUTPUT: %w", err)
	}
	if _, err := file.WriteString(values); err != nil {
		_ = file.Close()
		return fmt.Errorf("write GITHUB_OUTPUT: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close GITHUB_OUTPUT: %w", err)
	}
	return nil
}
