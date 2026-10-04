package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/desktop/internal/web"
	"github.com/colonyops/hive/cmd/desktop/internal/web/extractors"
)

type pushRequest struct {
	// Target is either a configured target's name (a JSON string) or an inline
	// {url, secret} object. Inline exists because the desktop's webhook port is
	// drawn at random per install (ADR local-webhook-listener), so no target can be committed to
	// config — an agent reads the base URL from Settings ▸ Webhooks and supplies
	// it here.
	Target targetRef `json:"target"`
	// Payload names a configured payload; Body supplies one inline. Exactly
	// one is required.
	Payload   string         `json:"payload,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
	Overrides map[string]any `json:"overrides,omitempty"`
}

func (r pushRequest) Validate() error {
	var errs criterio.FieldErrorsBuilder
	if (r.Payload == "") == (r.Body == nil) {
		errs = errs.Append("payload", errors.New("provide exactly one of payload or body"))
	}
	switch {
	case (r.Target.Name == "") == (r.Target.URL == ""):
		errs = errs.Append("target", errors.New("provide exactly one of a target name or a target url"))
	case r.Target.URL != "":
		if err := validateLoopbackURL(r.Target.URL); err != nil {
			errs = errs.Append("target", err)
		}
	}
	return errs.ToError()
}

// targetRef is a webhook target given either by name or inline. It decodes a
// JSON string into Name and a JSON object into URL/Secret, so one field accepts
// both forms without the caller choosing a key.
type targetRef struct {
	Name   string
	URL    string
	Secret string
}

func (t *targetRef) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		t.Name = name
		return nil
	}
	var obj struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&obj); err != nil {
		return fmt.Errorf("target must be a name or a {url, secret} object: %w", err)
	}
	t.URL, t.Secret = obj.URL, obj.Secret
	return nil
}

// deliveryError is the 502 body: the delivery attempt is the useful output
// even when it failed, so it rides along with the error.
type deliveryError struct {
	web.ErrorBody
	Result *PushResult `json:"result,omitempty"`
}

type pushResponse struct {
	Result PushResult `json:"result"`
}

func (c *Control) Push(w http.ResponseWriter, r *http.Request) error {
	req, err := extractors.Body[pushRequest](w, r)
	if err != nil {
		return err
	}

	payload, label := req.Body, "inline"
	if req.Payload != "" {
		// Payload returns a copy, so merging overrides here cannot rewrite the
		// stored payload — the next push of the same name is the same event.
		named, ok := c.pusher.Payload(req.Payload)
		if !ok {
			// A 502, matching an unknown target: both are the delivery failing
			// rather than a malformed request.
			return server.JSON(w, http.StatusBadGateway, deliveryError{
				Kind: "unavailable", Message: fmt.Sprintf("no payload named %q", req.Payload),
			})
		}
		payload, label = named, req.Payload
	}
	maps.Copy(payload, req.Overrides)

	var result PushResult
	if req.Target.Name != "" {
		result, err = c.pusher.Push(r.Context(), req.Target.Name, label, payload)
	} else {
		result, err = c.pusher.PushInline(r.Context(), req.Target.URL, req.Target.Secret, label, payload)
	}
	if err != nil {
		return server.JSON(w, http.StatusBadGateway, deliveryError{
			Kind: "unavailable", Message: err.Error(),
			Result: &result,
		})
	}
	return server.JSON(w, http.StatusOK, pushResponse{Result: result})
}

// validateLoopbackURL rejects an inline webhook target that is not a loopback
// http(s) URL. devserver POSTs a secret-bearing body to it, and the desktop's
// listener binds loopback only (ADR local-webhook-listener, 0017), so a non-loopback target is
// either a mistake or an attempt to make dev tooling reach off the machine.
func validateLoopbackURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid target url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("target url must be http:// or https://")
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errors.New("target url host must be loopback (127.0.0.1, ::1, or localhost)")
	}
	return nil
}
