package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/hay-kot/appkit/httpclient"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

// ErrForbidden is the relay refusing an email outright: not on its allowlist,
// a sign-in method it has turned off, or no password configured for it.
var ErrForbidden = errors.New("relay: forbidden")

// SignedIn is the device token a sign-in mints. Token is the plaintext, which
// the relay returns only in this response.
type SignedIn struct {
	Token     string    `json:"token"`
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Kind      string    `json:"kind"`
	Device    string    `json:"device"`
	CreatedAt time.Time `json:"created_at"`
}

// StartEmail asks the relay to mail email a sign-in code. Like every sign-in
// call it is meant for a client built without a token.
func (c *Client) StartEmail(ctx context.Context, email string) error {
	return c.signIn(ctx, "/v1/auth/start", map[string]string{"email": email}, nil)
}

func (c *Client) VerifyEmail(ctx context.Context, email, code, device string) (SignedIn, error) {
	var out SignedIn
	body := map[string]string{"email": email, "code": code, "device": device}
	return out, c.signIn(ctx, "/v1/auth/verify", body, &out)
}

func (c *Client) Login(ctx context.Context, email, password, device string) (SignedIn, error) {
	var out SignedIn
	body := map[string]string{"email": email, "password": password, "device": device}
	return out, c.signIn(ctx, "/v1/auth/login", body, &out)
}

// refusal keeps the relay's message as the error text, because the user acts
// on it ("invalid code; 2 attempts left"), while still matching its sentinel.
type refusal struct {
	sentinel error
	message  string
}

func (r *refusal) Error() string {
	if r.message == "" {
		return r.sentinel.Error()
	}
	return r.message
}

func (r *refusal) Unwrap() error { return r.sentinel }

func (c *Client) signIn(ctx context.Context, path string, in, out any) error {
	resp, err := c.api.Post(ctx, path, httpclient.JSONBody(in), httpclient.JSONContent())
	if err != nil {
		return c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return &refusal{sentinel: sourcehttp.ErrUnauthorized, message: readMessage(resp.Body)}
	case http.StatusForbidden:
		return &refusal{sentinel: ErrForbidden, message: readMessage(resp.Body)}
	}
	if err := c.errs.Status(resp); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return c.errs.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func readMessage(body io.Reader) string {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(body, 4096)).Decode(&payload)
	return payload.Message
}
