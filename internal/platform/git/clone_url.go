package git

import (
	"net"
	"net/url"
	"strings"

	"github.com/colonyops/hive/internal/domain/session"
)

// RepoLocation is a forge host and repository path that a clone URL can be built for.
type RepoLocation struct {
	// Host is the hostname plus a non-default port. Only HTTPS URLs use the port.
	Host string
	// Path is owner/name or group/subgroup/name, without ".git".
	Path string
}

// IsExplicitRemote reports whether repo is already a remote or a path rather
// than an owner/name pair: it contains "://", "@", ":" or "\", or starts with
// "/", "./", "../" or "~/".
func IsExplicitRemote(repo string) bool {
	if strings.ContainsAny(repo, `@:\`) {
		return true
	}
	for _, prefix := range []string{"/", "./", "../", "~/"} {
		if strings.HasPrefix(repo, prefix) {
			return true
		}
	}
	return false
}

// RemoteProtocol returns the transport of remote: https for http(s)://, ssh
// for ssh:// and SCP-style [user@]host:path. ok is false for anything else,
// such as a local path or git://.
func RemoteProtocol(remote string) (session.CloneProtocol, bool) {
	remote = strings.TrimSpace(remote)
	if scheme, _, found := strings.Cut(remote, "://"); found {
		switch scheme {
		case "https", "http":
			return session.CloneProtocolHttps, true
		case "ssh":
			return session.CloneProtocolSsh, true
		}
		return "", false
	}
	if _, _, ok := remoteHostPath(remote); ok {
		return session.CloneProtocolSsh, true
	}
	return "", false
}

// ItemRepoLocation expands an item's owner/repo using the host of its http(s)
// web URL. ok is false for an explicit remote, a malformed repo, or a URL
// without a host.
func ItemRepoLocation(repo, itemURL string) (RepoLocation, bool) {
	repo = strings.TrimSpace(repo)
	if repo == "" || IsExplicitRemote(repo) || strings.ContainsAny(repo, " \t\r\n?#") {
		return RepoLocation{}, false
	}
	path := strings.TrimSuffix(repo, ".git")
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return RepoLocation{}, false
	}
	for _, segment := range segments {
		if segment == "" {
			return RepoLocation{}, false
		}
	}

	u, err := url.Parse(strings.TrimSpace(itemURL))
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return RepoLocation{}, false
	}
	return RepoLocation{Host: webHost(u), Path: path}, true
}

// webHost returns the URL's host, dropping the scheme's default port.
func webHost(u *url.URL) string {
	port := u.Port()
	if port == "" || port == defaultRemotePorts[u.Scheme] {
		return bracketIPv6(u.Hostname())
	}
	return net.JoinHostPort(u.Hostname(), port)
}

func bracketIPv6(hostname string) string {
	if strings.Contains(hostname, ":") {
		return "[" + hostname + "]"
	}
	return hostname
}

// CloneURL renders https://host[:port]/path.git, or git@hostname:path.git for
// SSH. SSH never carries a port: a forge whose SSH port differs from its web
// port needs an explicit repo_template.
func (l RepoLocation) CloneURL(p session.CloneProtocol) string {
	if p == session.CloneProtocolSsh {
		hostname := l.Host
		if h, _, err := net.SplitHostPort(l.Host); err == nil {
			hostname = bracketIPv6(h)
		}
		return "git@" + hostname + ":" + l.Path + ".git"
	}
	return "https://" + l.Host + "/" + l.Path + ".git"
}

// Candidates returns the HTTPS and SSH URLs, so a rule pattern written for
// either transport matches.
func (l RepoLocation) Candidates() []string {
	return []string{l.CloneURL(session.CloneProtocolHttps), l.CloneURL(session.CloneProtocolSsh)}
}
