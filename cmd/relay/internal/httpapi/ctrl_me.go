package httpapi

import (
	"fmt"
	"net/http"

	"github.com/hay-kot/httpkit/server"
)

type meResponse struct {
	Email      string          `json:"email"`
	Encryption bool            `json:"encryption"`
	Limits     limitsResponse  `json:"limits"`
	Usage      usageResponse   `json:"usage"`
	Methods    methodsResponse `json:"methods"`
	Version    string          `json:"version"`
}

type limitsResponse struct {
	BodyBytes           int64 `json:"body_bytes"`
	HeaderBytes         int   `json:"header_bytes"`
	DeliveriesPerSource int   `json:"deliveries_per_source"`
	BytesPerAccount     int64 `json:"bytes_per_account"`
	SourcesPerAccount   int   `json:"sources_per_account"`
	RetentionSeconds    int64 `json:"retention_seconds"`
}

type usageResponse struct {
	Bytes   int64 `json:"bytes"`
	Sources int   `json:"sources"`
}

func (s *api) me(w http.ResponseWriter, r *http.Request) error {
	acct := accountFrom(r.Context())
	n, err := s.core.Store.Sources().CountByEmail(r.Context(), acct.Email)
	if err != nil {
		return fmt.Errorf("count sources: %w", err)
	}
	l := s.core.Limits
	return server.JSON(w, http.StatusOK, meResponse{
		Email:      acct.Email,
		Encryption: acct.Encryption,
		Limits: limitsResponse{
			BodyBytes:           l.BodyBytes,
			HeaderBytes:         l.HeaderBytes,
			DeliveriesPerSource: l.DeliveriesPerSource,
			BytesPerAccount:     l.BytesPerAccount,
			SourcesPerAccount:   l.SourcesPerAccount,
			RetentionSeconds:    int64(l.Retention.Seconds()),
		},
		Usage:   usageResponse{Bytes: acct.BytesUsed, Sources: n},
		Methods: signInMethods(),
		Version: s.core.Version,
	})
}
