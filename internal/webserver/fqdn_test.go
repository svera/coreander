package webserver

import "testing"

func TestResolveFQDN(t *testing.T) {
	tests := []struct {
		name string
		fqdn string
		port int
		want string
	}{
		{"default localhost gets port appended", "localhost", 3000, "localhost:3000"},
		{"localhost is case-insensitive", "LocalHost", 3000, "LocalHost:3000"},
		{"localhost with explicit port is untouched", "localhost:8080", 3000, "localhost:8080"},
		{"localhost inherits custom listening port", "localhost", 4000, "localhost:4000"},
		{"custom fqdn inherits listening port", "example.com", 4000, "example.com:4000"},
		{"LAN IP inherits listening port", "192.168.1.139", 4000, "192.168.1.139:4000"},
		{"custom fqdn explicit port is untouched", "example.com:8080", 4000, "example.com:8080"},
		{"LAN IP explicit port is untouched", "192.168.1.139:8080", 4000, "192.168.1.139:8080"},
		{"proxy HTTPS port overrides listening port", "example.com:443", 4000, "example.com:443"},
		{"proxy HTTP port overrides listening port", "example.com:80", 4000, "example.com:80"},
		{"bare IPv6 inherits listening port", "2001:db8::1", 4000, "[2001:db8::1]:4000"},
		{"bracketed IPv6 inherits listening port", "[2001:db8::1]", 4000, "[2001:db8::1]:4000"},
		{"IPv6 explicit port is untouched", "[2001:db8::1]:8080", 4000, "[2001:db8::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveFQDN(tt.fqdn, tt.port); got != tt.want {
				t.Errorf("ResolveFQDN(%q, %d) = %q, want %q", tt.fqdn, tt.port, got, tt.want)
			}
		})
	}
}
