package domain

import (
	"errors"
	"testing"
)

func TestNormalizeIPWhitelist(t *testing.T) {
	got, err := NormalizeIPWhitelist([]string{" 10.0.0.7 ", "10.0.0.7", "192.168.1.9/24", "", "::ffff:1.2.3.4", "2001:db8::1/64"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.7", "192.168.1.0/24", "1.2.3.4", "2001:db8::/64"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	for _, bad := range []string{"10.0.0.300", "10.0.0.0/33", "example.com", "1.2.3.4/abc"} {
		if _, err := NormalizeIPWhitelist([]string{bad}); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%q: expected ErrInvalidArgument, got %v", bad, err)
		}
	}
}

func TestAllowsIP(t *testing.T) {
	open := APIKey{}
	if !open.AllowsIP("8.8.8.8") {
		t.Fatal("empty whitelist must allow any source")
	}
	k := APIKey{IPWhitelist: []string{"10.1.2.3", "172.16.0.0/12", "2001:db8::/32"}}
	cases := map[string]bool{
		"10.1.2.3":        true,
		"10.1.2.4":        false,
		"172.20.9.9":      true,
		"172.32.0.1":      false,
		"::ffff:10.1.2.3": true, // IPv4-mapped IPv6 from a dual-stack listener
		"2001:db8:1::5":   true,
		"2001:db9::1":     false,
		"":                false,
		"not-an-ip":       false,
	}
	for ip, want := range cases {
		if got := k.AllowsIP(ip); got != want {
			t.Errorf("AllowsIP(%q) = %v, want %v", ip, got, want)
		}
	}
}
