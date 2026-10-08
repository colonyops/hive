package grafana

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/pkg/duration"
)

// MetricsConfig is a Grafana metrics source node's configuration. credential is
// a direct field, not promoted from an embedded base, because the
// provider-enforcement test scans a config's own tagged fields for it. There is
// deliberately no url field — the stack URL is bound to the account at connect
// time, so a node cannot pair a token with an arbitrary host.
type MetricsConfig struct {
	// A ref and never a token: flows/ is dotfiles-managed, so an embedded token
	// would be a token in a git repo.
	Credential    string `json:"credential"      yaml:"credential"      jsonschema:"title=Credential,description=The connected Grafana stack to fetch as, as 'grafana/<account>'."`
	DatasourceUID string `json:"datasource_uid"  yaml:"datasource_uid"  jsonschema:"title=Datasource UID,description=The uid of the Prometheus-compatible datasource to query."`
	Expr          string `json:"expr"            yaml:"expr"            jsonschema:"title=Query,description=A PromQL expression, e.g. 'up' or 'sum(rate(http_requests_total[5m]))'."`
	Title         string `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"title=Title,description=The feed item's title. Defaults to the query when empty."`
	// Interval is the floor between fetches, for a query too expensive to
	// run on every tick.
	Interval duration.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The source still only runs on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
}

func (c *MetricsConfig) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	if strings.TrimSpace(c.DatasourceUID) == "" {
		return fmt.Errorf("grafana source: datasource_uid is required")
	}
	if strings.TrimSpace(c.Expr) == "" {
		return fmt.Errorf("grafana source: expr is required")
	}
	return connector.ValidateInterval("grafana source", c.Interval)
}

func (c *MetricsConfig) CredentialRef() (credentials.Ref, error) {
	return parseGrafanaRef(c.Credential)
}

// parseGrafanaRef parses a "grafana/<account>" ref, rejecting a missing ref or
// one naming another provider at load rather than at fetch time. Shared by every
// Grafana config so the provider check is written once.
func parseGrafanaRef(credential string) (credentials.Ref, error) {
	if strings.TrimSpace(credential) == "" {
		return credentials.Ref{}, fmt.Errorf("grafana source: credential is required (e.g. %q)", Provider+"/grafana.example.com-1")
	}
	ref, err := credentials.ParseRef(credential)
	if err != nil {
		return credentials.Ref{}, fmt.Errorf("grafana source: %w", err)
	}
	if ref.Provider != Provider {
		return credentials.Ref{}, fmt.Errorf("grafana source: credential %q is not a %s credential", credential, Provider)
	}
	return ref, nil
}
