package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceAuth is the pending device authorization the UI presents: the user
// opens VerificationURI and enters UserCode.
type DeviceAuth struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// StartDeviceFlow requests a device + user code pair for the OAuth device
// flow. clientID is the public OAuth app client ID.
func (c *Client) StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (DeviceAuth, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", strings.Join(scopes, " "))

	var auth DeviceAuth
	if err := c.postForm(ctx, c.authBase+"/login/device/code", form, &auth); err != nil {
		return DeviceAuth{}, err
	}
	if auth.DeviceCode == "" || auth.UserCode == "" {
		return DeviceAuth{}, fmt.Errorf("github: device flow: empty codes in response")
	}
	if auth.Interval <= 0 {
		auth.Interval = 5
	}
	if auth.ExpiresIn <= 0 {
		// GitHub always sends expires_in (900); guard so a missing value
		// never yields an already-expired poll context.
		auth.ExpiresIn = 900
	}
	return auth, nil
}

// PollDeviceFlow polls the token endpoint until the user authorizes the
// device, the code expires, or ctx is canceled. It blocks; run it in a
// goroutine and honor ctx for cancellation.
func (c *Client) PollDeviceFlow(ctx context.Context, clientID string, auth DeviceAuth) (string, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("device_code", auth.DeviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	interval := time.Duration(auth.Interval) * time.Second

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		var token tokenResponse
		if err := c.postForm(ctx, c.authBase+"/login/oauth/access_token", form, &token); err != nil {
			return "", err
		}

		switch token.Error {
		case "":
			if token.AccessToken == "" {
				return "", fmt.Errorf("github: device flow: empty access token")
			}
			return token.AccessToken, nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return "", fmt.Errorf("github: device flow: code expired")
		case "access_denied":
			return "", fmt.Errorf("github: device flow: authorization denied")
		default:
			return "", fmt.Errorf("github: device flow: %s", token.Error)
		}
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := statusError(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("github: decode device flow response: %w", err)
	}
	return nil
}
