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

func runInteractiveRelease(ctx context.Context, dryRun bool) error {
	err := executeInteractiveRelease(ctx, dryRun)
	if errors.Is(err, errReleaseCancelled) || errors.Is(err, huh.ErrUserAborted) {
		zerolog.Ctx(ctx).Info().Msg("release cancelled")
		return nil
	}
	return err
}

func executeInteractiveRelease(ctx context.Context, dryRun bool) error {
	if dryRun {
		zerolog.Ctx(ctx).Info().Bool("dry_run", true).Msg("starting release preview; nothing will be published")
	}

	zerolog.Ctx(ctx).Info().Msg("refreshing release refs")
	if err := runReleaseCommand(ctx, "git", "fetch", "origin", "main", "--tags", "--prune"); err != nil {
		return err
	}
	if !dryRun {
		if err := quietCommand(ctx, "gh", "auth", "status"); err != nil {
			return fmt.Errorf("gh must be authenticated to tag the release and dispatch the publish workflow: %w", err)
		}
	}

	plan, err := planRelease(ctx, "", !dryRun)
	if err != nil {
		return err
	}
	printReleasePlanCard(plan)
	confirmed, err := confirmRelease(plan, dryRun)
	if err != nil {
		return err
	}
	if !confirmed {
		return errReleaseCancelled
	}
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
		row("Tag", plan.version.tag(), valueStyle),
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

func confirmRelease(plan releasePlan, dryRun bool) (bool, error) {
	title := "Publish " + plan.version.String() + "?"
	description := "The local workflow will run the release gates, build every desktop platform, sign and notarize macOS, upload the desktop artifacts, " +
		"push the tag, and dispatch the publish workflow that ships the CLI and creates the GitHub release."
	if dryRun {
		title = "Complete this dry run for " + plan.version.String() + "?"
		description = "This stops after confirmation. It will not run gates, build artifacts, upload anything, push a tag, or dispatch a workflow."
	}
	confirmed := false
	err := huh.NewConfirm().
		Title(title).
		Description(description).
		Affirmative("Publish").
		Negative("Cancel").
		Value(&confirmed).
		Run()
	if err != nil {
		return false, fmt.Errorf("confirm release: %w", err)
	}
	return confirmed, nil
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
