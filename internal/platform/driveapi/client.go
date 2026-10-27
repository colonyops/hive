package driveapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hay-kot/appkit/httpclient"

	"github.com/colonyops/hive/internal/domain/drive"
)

const (
	requestTimeout  = 60 * time.Second
	maxResponseSize = 64 << 20
)

// APIError is a non-2xx response. Body is zero when the server sent no JSON
// error body, for example from a proxy in front of it.
type APIError struct {
	Status int
	Body   ErrorBody
}

func (e *APIError) Error() string {
	if e.Body.Message == "" {
		return fmt.Sprintf("drive server: %s", http.StatusText(e.Status))
	}
	return fmt.Sprintf("drive server: %s (%s)", e.Body.Message, e.Body.Kind)
}

// Client calls the drive server API as one token holder.
type Client struct {
	http *httpclient.Client
}

// NewClient returns a client for server, a URL or host[:port]. token is read
// on every request, so a rotated credential applies without a new client.
func NewClient(server string, token func() string) (*Client, error) {
	base, err := baseURL(server)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: requestTimeout}
	return &Client{http: httpclient.New(hc, base, httpclient.BearerAuth(token))}, nil
}

// Host is the normalized host[:port] of server. It names the stored
// credential and the upload records, so one server has one spelling.
func Host(server string) (string, error) {
	u, err := parseServer(server)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func baseURL(server string) (string, error) {
	u, err := parseServer(server)
	if err != nil {
		return "", err
	}
	scheme := "https"
	if isLoopback(u.Hostname()) {
		scheme = "http"
	}
	return scheme + "://" + u.Host, nil
}

// parseServer accepts "host[:port]" or a URL. Any scheme is dropped: the
// scheme follows from the host, so a token can never be sent over plain
// http to a remote server.
func parseServer(server string) (*url.URL, error) {
	raw := strings.TrimSpace(server)
	if raw == "" {
		return nil, errors.New("drive server is empty")
	}
	if scheme, _, found := strings.Cut(raw, "://"); found {
		if s := strings.ToLower(scheme); s != "http" && s != "https" {
			return nil, fmt.Errorf("drive server %q: scheme must be http or https", server)
		}
	} else {
		raw = "//" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("drive server %q: %w", server, err)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("drive server %q has no host", server)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("drive server %q must be a host, with no path, user, or query", server)
	}
	if strings.EqualFold(u.Scheme, "http") && !isLoopback(u.Hostname()) {
		return nil, fmt.Errorf("drive server %q: plain http is only allowed for localhost", server)
	}
	u.Host = strings.ToLower(u.Host)
	return u, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) Me(ctx context.Context) (Me, error) {
	resp, err := c.http.Get(ctx, "/api/v1/me")
	if err != nil {
		return Me{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	return decode[Me](resp)
}

// Put stores body at path, replacing any document there.
func (c *Client) Put(ctx context.Context, space drive.Space, path string, body []byte) (Document, error) {
	resp, err := c.http.Put(ctx, DocPath(space, path), bytes.NewReader(body))
	if err != nil {
		return Document{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	return decode[Document](resp)
}

// Get returns the document body. The server sends the body alone, so the
// returned Document holds what the client can know: space, path, kind, size,
// and the body's sha256.
func (c *Client) Get(ctx context.Context, space drive.Space, path string) (Document, []byte, error) {
	resp, err := c.http.Get(ctx, DocPath(space, path))
	if err != nil {
		return Document{}, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return Document{}, nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return Document{}, nil, fmt.Errorf("read document: %w", err)
	}
	sum := sha256.Sum256(body)
	return Document{
		Space:  space.Key,
		Path:   path,
		Kind:   drive.KindOf(path),
		SHA256: hex.EncodeToString(sum[:]),
		Size:   int64(len(body)),
	}, body, nil
}

func (c *Client) List(ctx context.Context, space drive.Space) ([]Document, error) {
	resp, err := c.http.Get(ctx, SpacePrefix(space))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return decode[[]Document](resp)
}

// Share creates the document's public link, or returns the existing one.
// expiresIn is "", "1d", "7d", "30d", or "none". "" leaves an existing link's expiry
// unchanged and gives a new link none. "none" clears the expiry.
func (c *Client) Share(ctx context.Context, space drive.Space, path, expiresIn string) (Share, error) {
	resp, err := c.http.Post(ctx, SpacePrefix(space)+"/shares/"+escapePath(path),
		httpclient.JSONBody(ShareRequest{ExpiresIn: expiresIn}), httpclient.JSONContent())
	if err != nil {
		return Share{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	return decode[Share](resp)
}

// The caller closes the body.
func decode[T any](resp *http.Response) (T, error) {
	var zero T
	if err := checkStatus(resp); err != nil {
		return zero, err
	}
	var out T
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&out); err != nil {
		return zero, fmt.Errorf("decode drive server response: %w", err)
	}
	return out, nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	apiErr := &APIError{Status: resp.StatusCode}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if json.Unmarshal(data, &apiErr.Body) != nil {
		apiErr.Body = ErrorBody{}
	}
	return apiErr
}
