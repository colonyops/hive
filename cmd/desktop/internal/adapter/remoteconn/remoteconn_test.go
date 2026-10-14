package remoteconn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestTLSIdentityPersists(t *testing.T) {
	dir := t.TempDir()
	first, public, err := certificate(dir)
	require.NoError(t, err)
	second, again, err := certificate(dir)
	require.NoError(t, err)
	require.Equal(t, first.Certificate, second.Certificate)
	require.Equal(t, public, again)
	require.NotContains(t, public, "PRIVATE KEY")
	info, err := os.Stat(filepath.Join(dir, "identity.pem"))
	require.NoError(t, err)
	require.EqualValues(t, 0o600, info.Mode().Perm())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "identity.pem"), []byte("broken"), 0o600))
	_, _, err = certificate(dir)
	require.Error(t, err)
}

func TestServerExportsVerifiedHTTPSOnly(t *testing.T) {
	dir := t.TempDir()
	server := &Server{Address: "127.0.0.1:0", Directory: dir, ConnectionFile: filepath.Join(dir, "connection.json"), Token: "secret", Logger: zerolog.Nop(), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	require.NoError(t, server.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	data, err := os.ReadFile(server.ConnectionFile)
	require.NoError(t, err)
	var credentials Credentials
	require.NoError(t, json.Unmarshal(data, &credentials))
	require.Equal(t, "secret", credentials.Token)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM([]byte(credentials.Certificate)))
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get(credentials.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response, err = http.Get(credentials.URL)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.Error(t, err, "untrusted TLS must fail")
	response, err = client.Get("http" + credentials.URL[len("https"):])
	if err == nil {
		require.NotEqual(t, http.StatusNoContent, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, server.Close())
	require.NoError(t, server.Close())
}

func TestRemoteClientRejectsUntrustedCertificate(t *testing.T) {
	pair, public, err := certificate(t.TempDir())
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	client, err := clientFor(Profile{Host: "example"}, Credentials{URL: server.URL, Token: "secret", Certificate: public})
	require.NoError(t, err)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	transport.DialContext = (&net.Dialer{}).DialContext
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	client.CloseIdleConnections()
	_, wrong, err := certificate(t.TempDir())
	require.NoError(t, err)
	client, err = clientFor(Profile{Host: "example"}, Credentials{URL: server.URL, Token: "secret", Certificate: wrong})
	require.NoError(t, err)
	transport, ok = client.Transport.(*http.Transport)
	require.True(t, ok)
	transport.DialContext = (&net.Dialer{}).DialContext
	response, err = client.Get(server.URL)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.Error(t, err)
	client.CloseIdleConnections()
}

func TestConnectionPathAndCancellation(t *testing.T) {
	c := &connection{done: make(chan struct{})}
	for _, path := range []string{"https://other/api/terminal/attach", "/api/status", "/api/terminal/../status", "/api/terminal/%2e%2e/status", "/api/terminal/attach?token=x"} {
		_, err := c.request(t.Context(), path, "{}")
		require.Error(t, err)
	}
	close(c.done)
	manager := NewManager()
	manager.Close()
	manager.Close()
	_, err := manager.Request(t.Context(), "missing", "/api/terminal/attach", "{}")
	require.Error(t, err)
	require.NoError(t, manager.CloseStream("missing", "missing"))
}

func TestSSHProfileValidationAndQuoting(t *testing.T) {
	for _, host := range []string{"-oProxyCommand=evil", "host;evil", "host\nevil", "user host", ""} {
		require.Error(t, (Profile{Host: host, ConnectionFile: "~/connection.json"}).Validate())
	}
	require.NoError(t, (Profile{Host: "user@my-server", ConnectionFile: "~/hive's files/connection.json"}).Validate())
	require.Equal(t, "'hive'\\''s files'", quote("hive's files"))
}

func TestTokenRotationRetriesOnlyUnauthorized(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer new" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	loads := 0
	c := &connection{
		done: make(chan struct{}), credentials: Credentials{URL: server.URL, Token: "old"}, client: server.Client(), streams: map[string]*stream{},
		loadCredentials: func(context.Context, Profile) (Credentials, *http.Client, error) {
			loads++
			return Credentials{URL: server.URL, Token: "new"}, server.Client(), nil
		},
	}
	defer c.close()
	response, err := c.request(t.Context(), "/api/terminal/attach", "{}")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.Status)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, loads)
	response, err = c.request(t.Context(), "/api/terminal/attach", "{}")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.Status)
	require.Equal(t, 3, calls)
	require.Equal(t, 1, loads)
}
