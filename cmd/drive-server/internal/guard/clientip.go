package guard

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP strips the port from r.RemoteAddr (netip.ParseAddrPort) and, when
// that address is inside one of trusted, walks X-Forwarded-For from the right
// and returns the first entry that is not a trusted proxy. With no trusted
// proxies the header is ignored, so a sender cannot pick its own bucket.
//
// It returns the zero Addr when RemoteAddr does not parse; callers treat that
// as one shared bucket.
func ClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer := parsePeer(r.RemoteAddr)
	if !peer.IsValid() || !inAny(peer, trusted) {
		return peer
	}

	var entries []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		entries = append(entries, strings.Split(v, ",")...)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(entries[i]))
		if err != nil {
			return peer
		}
		addr = addr.Unmap()
		if !inAny(addr, trusted) {
			return addr
		}
	}
	return peer
}

func parsePeer(remote string) netip.Addr {
	if ap, err := netip.ParseAddrPort(remote); err == nil {
		return ap.Addr().Unmap()
	}
	if a, err := netip.ParseAddr(remote); err == nil {
		return a.Unmap()
	}
	return netip.Addr{}
}

func inAny(a netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ParseProxies parses CIDRs, or bare addresses, naming the proxies whose
// X-Forwarded-For ClientIP honours.
func ParseProxies(cidrs []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, c := range cidrs {
		if p, err := netip.ParsePrefix(c); err == nil {
			out = append(out, p)
			continue
		}
		a, err := netip.ParseAddr(c)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q is not a CIDR or an address", c)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}
