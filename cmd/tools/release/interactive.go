package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/rs/zerolog"
)

var errReleaseCancelled = errors.New("release cancelled")

type releaseDecision string

type releaseBump string

const (
	decisionPublish releaseDecision = "publish"
	decisionVersion releaseDecision = "version"
	decisionCancel  releaseDecision = "cancel"

	bumpPatch releaseBump = "patch"
	bumpMinor releaseBump = "minor"
	bumpMajor releaseBump = "major"
)

func parseInteractiveReleaseArgs(args []string) (string, string, error) {
	if len(args) > 2 {
		return "", "", errors.New("expected an optional channel (dev, beta, or stable) and optional version")
	}
	if len(args) == 0 {
		return "", "", nil
	}
	if !validChannel(args[0]) {
		return "", "", errors.New("expected a channel: dev, beta, or stable")
	}
	if len(args) == 1 {
		return args[0], "", nil
	}
	return args[0], args[1], nil
}

func runInteractiveRelease(ctx context.Context, channel, candidate string, dryRun bool) error {
	err := executeInteractiveRelease(ctx, channel, candidate, dryRun)
	if errors.Is(err, errReleaseCancelled) || errors.Is(err, huh.ErrUserAborted) {
		zerolog.Ctx(ctx).Info().Msg("release cancelled")
		return nil
	}
	return err
}

func executeInteractiveRelease(ctx context.Context, channel, candidate string, dryRun bool) error {
	if dryRun {
		zerolog.Ctx(ctx).Info().Bool("dry_run", true).Msg("starting release preview; nothing will be published")
	}
	if channel == "" {
		selected, err := promptReleaseChannel()
		if err != nil {
			return err
		}
		channel = selected
	}

	zerolog.Ctx(ctx).Info().Msg("refreshing release refs")
	if err := runReleaseCommand(ctx, "git", "fetch", "origin", "main", "--tags", "--prune"); err != nil {
		return err
	}
	if !dryRun {
		if err := quietCommand(ctx, "gh", "auth", "status"); err != nil {
			return fmt.Errorf("gh must be authenticated to record the GitHub release: %w", err)
		}
	}

	preparePlan := prepareReleasePlan
	if dryRun {
		preparePlan = previewReleasePlan
	}
	plan, err := preparePlan(ctx, channel, candidate)
	if err != nil {
		return err
	}
	if candidate == "" {
		version, err := promptReleaseBump(plan.version)
		if err != nil {
			return err
		}
		if version.String() != plan.version.String() {
			plan, err = preparePlan(ctx, channel, version.String())
			if err != nil {
				return err
			}
		}
	}
	for {
		printReleasePlanCard(plan)
		decision, err := promptReleaseDecision(plan, dryRun)
		if err != nil {
			return err
		}
		switch decision {
		case decisionCancel:
			return errReleaseCancelled
		case decisionVersion:
			candidate, err := promptReleaseVersion(plan)
			if err != nil {
				return err
			}
			plan, err = preparePlan(ctx, channel, candidate)
			if err != nil {
				return err
			}
		case decisionPublish:
			if dryRun {
				zerolog.Ctx(ctx).Info().Str("version", plan.version.String()).Msg("dry run complete; nothing was published")
				return nil
			}
			if err := runReleaseGates(ctx); err != nil {
				return err
			}
			if err := validateConfirmedReleasePlan(ctx, plan); err != nil {
				return err
			}
			return publish(ctx, []string{plan.version.String()})
		default:
			return fmt.Errorf("unknown release decision %q", decision)
		}
	}
}

func printReleasePlanCard(plan releasePlan) {
	_, _ = fmt.Fprintln(os.Stderr, "\n"+renderReleasePlan(plan)+"\n")
}

func renderReleasePlan(plan releasePlan) string {
	accent := lipgloss.Color("#C5ADF9")
	labelStyle := lipgloss.NewStyle().Width(15).Foreground(lipgloss.Color("#888888"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
	versionStyle := valueStyle.Bold(true).Foreground(accent)
	row := func(label, value string, style lipgloss.Style) string {
		return labelStyle.Render(label) + style.Render(value)
	}
	current := func(channel string) string {
		if version := plan.currentManifests[channel]; version != "" {
			return version
		}
		return "empty"
	}
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(accent).Render("Release candidate"),
		"",
		row("Version", plan.version.String(), versionStyle),
		row("Channel", plan.channel, valueStyle),
		row("Publishes to", strings.Join(plan.version.affectedChannels(), " + "), valueStyle),
		row("Commit", plan.commit, valueStyle),
		row("Subject", plan.subject, valueStyle),
		"",
		row("Current stable", current("stable"), valueStyle),
		row("Current beta", current("beta"), valueStyle),
		row("Current dev", current("dev"), valueStyle),
	}, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 1).
		Render(content)
}

func promptReleaseChannel() (string, error) {
	channel := "dev"
	err := huh.NewSelect[string]().
		Title("Release channel").
		Description("Select which channel to publish. More stable releases also update the less-stable manifests.").
		Options(
			huh.NewOption("dev — update dev", "dev"),
			huh.NewOption("beta — update beta + dev", "beta"),
			huh.NewOption("stable — update stable + beta + dev", "stable"),
		).
		Value(&channel).
		Run()
	if err != nil {
		return "", fmt.Errorf("select release channel: %w", err)
	}
	return channel, nil
}

func promptReleaseBump(candidate releaseVersion) (releaseVersion, error) {
	bump := bumpPatch
	err := huh.NewSelect[releaseBump]().
		Title("Version increment").
		Description("Patch follows the normal channel progression; minor and major start a new base version.").
		Options(
			huh.NewOption("Patch — "+releaseVersionForBump(candidate, bumpPatch).String(), bumpPatch),
			huh.NewOption("Minor — "+releaseVersionForBump(candidate, bumpMinor).String(), bumpMinor),
			huh.NewOption("Major — "+releaseVersionForBump(candidate, bumpMajor).String(), bumpMajor),
		).
		Value(&bump).
		Run()
	if err != nil {
		return releaseVersion{}, fmt.Errorf("select release version increment: %w", err)
	}
	return releaseVersionForBump(candidate, bump), nil
}

func releaseVersionForBump(candidate releaseVersion, bump releaseBump) releaseVersion {
	if bump == bumpPatch {
		return candidate
	}
	version := releaseVersion{base: candidate.base}
	switch bump {
	case bumpMinor:
		version.base.minor++
		version.base.patch = 0
	case bumpMajor:
		version.base.major++
		version.base.minor = 0
		version.base.patch = 0
	default:
		return candidate
	}
	if candidate.channel() != "stable" {
		version.prerelease = candidate.channel()
		version.number = 1
	}
	return version
}

func promptReleaseDecision(plan releasePlan, dryRun bool) (releaseDecision, error) {
	title := "Publish this public release?"
	description := "The local workflow will run release gates, build every platform, sign and notarize macOS, upload public artifacts, push the tag, and create a GitHub release."
	publishLabel := "Publish " + plan.version.String()
	if dryRun {
		title = "Complete this dry run?"
		description = "This stops after confirmation. It will not run gates, build artifacts, upload anything, push a tag, or create a GitHub release."
		publishLabel = "Finish dry run for " + plan.version.String()
	}
	decision := decisionCancel
	err := huh.NewSelect[releaseDecision]().
		Title(title).
		Description(description).
		Options(
			huh.NewOption(publishLabel, decisionPublish),
			huh.NewOption("Choose another version", decisionVersion),
			huh.NewOption("Cancel", decisionCancel),
		).
		Value(&decision).
		Run()
	if err != nil {
		return "", fmt.Errorf("confirm release: %w", err)
	}
	return decision, nil
}

func promptReleaseVersion(plan releasePlan) (string, error) {
	candidate := plan.version.String()
	err := huh.NewInput().
		Title("Release version").
		Description("Enter a version for the " + plan.channel + " channel. It will be revalidated against live manifests and tags.").
		Value(&candidate).
		Validate(func(value string) error {
			return validateInteractiveReleaseVersion(plan.channel, value)
		}).
		Run()
	if err != nil {
		return "", fmt.Errorf("choose release version: %w", err)
	}
	return strings.TrimSpace(candidate), nil
}

func validateInteractiveReleaseVersion(channel, value string) error {
	version, err := parsePublishVersion(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	if version.channel() != channel {
		return fmt.Errorf("version belongs to %s, not %s", version.channel(), channel)
	}
	return nil
}

func runReleaseGates(ctx context.Context) error {
	zerolog.Ctx(ctx).Info().Msg("running release gates")
	for _, args := range [][]string{{"check"}, {"desktop:frontend:test"}} {
		if err := runReleaseCommand(ctx, "mi", args...); err != nil {
			return err
		}
	}
	return nil
}

func validateConfirmedReleasePlan(ctx context.Context, plan releasePlan) error {
	zerolog.Ctx(ctx).Info().Str("commit", plan.commit).Msg("revalidating confirmed release source")
	if err := validatePrepareSource(ctx); err != nil {
		return err
	}
	head, err := gitHead(ctx)
	if err != nil {
		return err
	}
	if head != plan.commit {
		return fmt.Errorf("confirmed release commit changed from %s to %s", plan.commit, head)
	}
	return nil
}

func runReleaseCommand(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(append([]string{name}, args...), " "), err)
	}
	return nil
}
