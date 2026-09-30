// Package posthog is the PostHog source connector: the error-tracking and
// insight-alert source nodes and the project auth that connects a project.
package posthog

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
)

// Provider is the credentials provider every PostHog credential is filed
// under. A ref is "posthog/<host>-<projectID>", so several projects on one
// host — separate products, or dev and prod — stay isolated and can be routed
// to different feeds.
const Provider = "posthog"

const SourceKind = "posthog"

// parsePostHogRef parses a "posthog/<account>" ref, rejecting a missing ref or
// one naming another provider at load rather than at fetch time. Shared by
// every PostHog config so the provider check is written once.
func parsePostHogRef(credential string) (credentials.Ref, error) {
	if strings.TrimSpace(credential) == "" {
		return credentials.Ref{}, fmt.Errorf("posthog source: credential is required (e.g. %q)", Provider+"/us.posthog.com-1")
	}
	ref, err := credentials.ParseRef(credential)
	if err != nil {
		return credentials.Ref{}, fmt.Errorf("posthog source: %w", err)
	}
	if ref.Provider != Provider {
		return credentials.Ref{}, fmt.Errorf("posthog source: credential %q is not a %s credential", credential, Provider)
	}
	return ref, nil
}
