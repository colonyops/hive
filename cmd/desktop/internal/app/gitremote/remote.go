package gitremote

import (
	"net/url"
	"strings"
)

// Resolve expands a canonical owner/repository using the item's host. Explicit
// remotes and paths are never converted to another protocol.
func Resolve(repo, itemURL, protocol string) string {
	if repo == "" || strings.ContainsAny(repo, ":@\\") || strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, "./") || strings.HasPrefix(repo, "../") || strings.HasPrefix(repo, "~/") {
		return repo
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repo, " \t\r\n?#") {
		return ""
	}
	u, err := url.Parse(itemURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	path := strings.TrimSuffix(repo, ".git") + ".git"
	if protocol == "ssh" {
		if u.Port() != "" || strings.Contains(u.Hostname(), ":") {
			return "ssh://git@" + u.Host + "/" + path
		}
		return "git@" + u.Host + ":" + path
	}
	return "https://" + u.Host + "/" + path
}
