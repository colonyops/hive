package relay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hay-kot/appkit/httpclient"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

const sourceName = "relay"

// Client is the relay's /v1 API. Error bodies are the relay's {kind, message}
// shape, which sourcehttp already reads the message out of.
type Client struct {
	api  *httpclient.Client
	errs sourcehttp.Errors
}

// NewClient builds a client for one relay. token is read per request so a
// rotated key applies without a rebuild; version is the desktop's own build
// version, sent in the User-Agent.
func NewClient(base string, token func() string, version string, log zerolog.Logger) *Client {
	return &Client{
		api: sourcehttp.New(sourcehttp.Config{
			Name:    sourceName,
			BaseURL: strings.TrimRight(base, "/"),
			Token:   token,
			Logger:  log,
		},
			httpclient.Header("Accept", "application/json"),
			httpclient.Header("User-Agent", "hive-desktop/"+version),
		),
		errs: sourcehttp.Errors{Name: sourceName},
	}
}

type Limits struct {
	BodyBytes           int64 `json:"body_bytes"`
	HeaderBytes         int   `json:"header_bytes"`
	DeliveriesPerSource int   `json:"deliveries_per_source"`
	BytesPerAccount     int64 `json:"bytes_per_account"`
	SourcesPerAccount   int   `json:"sources_per_account"`
	RetentionSeconds    int64 `json:"retention_seconds"`
}

type Usage struct {
	Bytes   int64 `json:"bytes"`
	Sources int   `json:"sources"`
}

type Methods struct {
	EmailCode bool `json:"email_code"`
	Password  bool `json:"password"`
	APIKey    bool `json:"api_key"`
}

// Me is the account a key authenticates as.
type Me struct {
	Email      string  `json:"email"`
	Encryption bool    `json:"encryption"`
	Limits     Limits  `json:"limits"`
	Usage      Usage   `json:"usage"`
	Methods    Methods `json:"methods"`
	Version    string  `json:"version"`
}

// SourceView is one relay source. IngressURL is set only when the source was
// just created; the relay never returns it again.
type SourceView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	HasSecret  bool      `json:"has_secret"`
	CreatedAt  time.Time `json:"created_at"`
	Deliveries int       `json:"deliveries"`
	Bytes      int64     `json:"bytes"`
	IngressURL string    `json:"ingress_url,omitempty"`
}

// Page is one page of a source's delivery log. Next is the cursor to pass as
// after; it is empty when the page is.
type Page struct {
	Deliveries []DeliveryView `json:"deliveries"`
	Next       string         `json:"next"`
	More       bool           `json:"more"`
}

// DeliveryView is one stored request. Body holds text or JSON as-is and
// BodyBase64 anything else; a sealed delivery carries only Enc and Blob.
type DeliveryView struct {
	ID            string              `json:"id"`
	ReceivedAt    time.Time           `json:"received_at"`
	Method        string              `json:"method,omitempty"`
	Path          string              `json:"path,omitempty"`
	Query         url.Values          `json:"query,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          string              `json:"body,omitempty"`
	BodyBase64    string              `json:"body_b64,omitempty"`
	ContentType   string              `json:"content_type,omitempty"`
	ContentLength int64               `json:"content_length"`
	Enc           string              `json:"enc,omitempty"`
	Blob          string              `json:"blob,omitempty"`
}

func (c *Client) Me(ctx context.Context) (Me, error) {
	var out Me
	return out, c.call(ctx, http.MethodGet, "/v1/me", nil, &out)
}

// Methods answers without a key.
func (c *Client) Methods(ctx context.Context) (Methods, error) {
	var out Methods
	return out, c.call(ctx, http.MethodGet, "/v1/auth/methods", nil, &out)
}

func (c *Client) ListSources(ctx context.Context) ([]SourceView, error) {
	var out struct {
		Sources []SourceView `json:"sources"`
	}
	if err := c.call(ctx, http.MethodGet, "/v1/sources", nil, &out); err != nil {
		return nil, err
	}
	return out.Sources, nil
}

func (c *Client) CreateSource(ctx context.Context, name, secret string) (SourceView, error) {
	var out SourceView
	body := map[string]string{"name": name, "secret": secret}
	return out, c.call(ctx, http.MethodPost, "/v1/sources", body, &out)
}

// RotateSource replaces the source's ingress token and returns the new URL;
// the old one stops accepting deliveries.
func (c *Client) RotateSource(ctx context.Context, id string) (string, error) {
	var out struct {
		IngressURL string `json:"ingress_url"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/sources/"+url.PathEscape(id)+"/rotate", nil, &out); err != nil {
		return "", err
	}
	return out.IngressURL, nil
}

func (c *Client) DeleteSource(ctx context.Context, id string) error {
	return c.call(ctx, http.MethodDelete, "/v1/sources/"+url.PathEscape(id), nil, nil)
}

// Deliveries returns the deliveries after the cursor, oldest first. limit 0
// takes the relay's default.
func (c *Client) Deliveries(ctx context.Context, sourceID, after string, limit int) (Page, error) {
	q := url.Values{}
	if after != "" {
		q.Set("after", after)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/v1/sources/" + url.PathEscape(sourceID) + "/deliveries"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out Page
	return out, c.call(ctx, http.MethodGet, path, nil, &out)
}

// CreateToken mints an API key on the account and returns its plaintext,
// which the relay never shows again, and its id for revoking it.
func (c *Client) CreateToken(ctx context.Context, name string) (plaintext, id string, err error) {
	var out struct {
		Token string `json:"token"`
		ID    string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/tokens", map[string]string{"name": name}, &out); err != nil {
		return "", "", err
	}
	return out.Token, out.ID, nil
}

func (c *Client) RevokeToken(ctx context.Context, id string) error {
	return c.call(ctx, http.MethodDelete, "/v1/tokens/"+url.PathEscape(id), nil, nil)
}

func (c *Client) call(ctx context.Context, method, path string, in, out any) error {
	var (
		resp *http.Response
		err  error
	)
	switch method {
	case http.MethodGet:
		resp, err = c.api.Get(ctx, path)
	case http.MethodPost:
		var body io.Reader = http.NoBody
		if in != nil {
			body = httpclient.JSONBody(in)
		}
		resp, err = c.api.Post(ctx, path, body, httpclient.JSONContent())
	case http.MethodDelete:
		resp, err = c.api.Delete(ctx, path)
	default:
		return c.errs.Errorf("unsupported method %s", method)
	}
	if err != nil {
		return c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

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
