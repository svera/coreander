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
		{"custom fqdn is untouched", "example.com", 3000, "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveFQDN(tt.fqdn, tt.port); got != tt.want {
				t.Errorf("ResolveFQDN(%q, %d) = %q, want %q", tt.fqdn, tt.port, got, tt.want)
			}
		})
	}
}
