package daemon_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mondaycom/mcli/internal/daemon"
)

func TestNewTunnel(t *testing.T) {
	t.Parallel()
	tun := daemon.NewTunnel(8420)
	if tun == nil {
		t.Fatal("NewTunnel returned nil")
	}
	if tun.URL() != "" {
		t.Errorf("expected empty URL before start, got %q", tun.URL())
	}
	if tun.URLChanges() == nil {
		t.Error("URLChanges() should return a non-nil channel")
	}
}

func TestParseCloudflaredURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		want string
	}{
		{
			line: `2024-01-01T00:00:00Z INF |  https://abc-def-ghi.trycloudflare.com`,
			want: "https://abc-def-ghi.trycloudflare.com",
		},
		{
			line: `INF Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):  https://my-tunnel-123.trycloudflare.com`,
			want: "https://my-tunnel-123.trycloudflare.com",
		},
		{
			line: `INF | https://tunnel-with-many-parts-here.trycloudflare.com`,
			want: "https://tunnel-with-many-parts-here.trycloudflare.com",
		},
		{
			// URL with path components — the regex stops at whitespace.
			line: "https://something.trycloudflare.com trailing text",
			want: "https://something.trycloudflare.com",
		},
		{
			line: "no url here",
			want: "",
		},
		{
			line: "https://example.com is not a cloudflare tunnel",
			want: "",
		},
		{
			line: "",
			want: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(strings.TrimSpace(tc.line[:min(len(tc.line), 40)]), func(t *testing.T) {
			t.Parallel()
			got := daemon.ParseCloudflaredURL(tc.line)
			if got != tc.want {
				t.Errorf("parseCloudflaredURL(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestTunnel_StartErrorWhenCloudflaredMissing(t *testing.T) {
	// Remove cloudflared from PATH to simulate a missing binary.
	t.Setenv("PATH", "")

	tun := daemon.NewTunnel(18420)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := tun.Start(ctx)
	if err == nil {
		t.Fatal("expected error when cloudflared is not in PATH")
	}
	if !strings.Contains(err.Error(), "cloudflared not found in PATH") {
		t.Errorf("unexpected error message: %v", err)
	}
}

