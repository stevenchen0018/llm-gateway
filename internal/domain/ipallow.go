package domain

import (
	"fmt"
	"net/netip"
	"strings"
)

// MaxIPWhitelist bounds how many entries one key may carry.
const MaxIPWhitelist = 100

// NormalizeIPWhitelist validates entries (single IPv4/IPv6 addresses or
// CIDR ranges), canonicalises them and removes duplicates.
func NormalizeIPWhitelist(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	seen := map[string]bool{}
	for _, raw := range entries {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		var canon string
		if strings.Contains(e, "/") {
			p, err := netip.ParsePrefix(e)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid CIDR %q", ErrInvalidArgument, e)
			}
			canon = p.Masked().String()
		} else {
			a, err := netip.ParseAddr(e)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid IP address %q", ErrInvalidArgument, e)
			}
			canon = a.Unmap().String()
		}
		if !seen[canon] {
			seen[canon] = true
			out = append(out, canon)
		}
	}
	if len(out) > MaxIPWhitelist {
		return nil, fmt.Errorf("%w: at most %d whitelist entries", ErrInvalidArgument, MaxIPWhitelist)
	}
	return out, nil
}

// AllowsIP reports whether a request from ip may use the key. An empty
// whitelist allows every source; an unparsable client address is denied
// when a whitelist is configured.
func (k *APIKey) AllowsIP(ip string) bool {
	if len(k.IPWhitelist) == 0 {
		return true
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, e := range k.IPWhitelist {
		if strings.Contains(e, "/") {
			if p, err := netip.ParsePrefix(e); err == nil && p.Contains(addr) {
				return true
			}
			continue
		}
		if a, err := netip.ParseAddr(e); err == nil && a.Unmap() == addr {
			return true
		}
	}
	return false
}
