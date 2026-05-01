package router_test

import (
	"testing"

	"github.com/configkits/mcp-gateway/internal/config"
	"github.com/configkits/mcp-gateway/internal/router"
)

func makeRouter(t *testing.T, servers []config.ServerConfig) *router.Router {
	t.Helper()
	r, err := router.New(servers)
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return r
}

func TestPrefixRouting(t *testing.T) {
	r := makeRouter(t, []config.ServerConfig{
		{Name: "github", Routing: &config.RoutingConfig{Prefix: "github_"}},
		{Name: "fs", Routing: &config.RoutingConfig{Prefix: "fs_"}},
	})

	cases := []struct {
		tool string
		want string
	}{
		{"github_create_issue", "github"},
		{"github_list_prs", "github"},
		{"fs_read_file", "fs"},
		{"unknown_tool", ""},
	}

	for _, tc := range cases {
		res, err := r.Route(tc.tool)
		if err != nil {
			t.Errorf("Route(%q) error: %v", tc.tool, err)
			continue
		}
		got := ""
		if res != nil {
			got = res.ServerName
		}
		if got != tc.want {
			t.Errorf("Route(%q) = %q, want %q", tc.tool, got, tc.want)
		}
	}
}

func TestPatternRouting(t *testing.T) {
	r := makeRouter(t, []config.ServerConfig{
		{Name: "search", Routing: &config.RoutingConfig{Pattern: `^search_.*`}},
	})

	res, err := r.Route("search_web")
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ServerName != "search" {
		t.Errorf("expected search, got %v", res)
	}

	res2, _ := r.Route("unrelated_tool")
	if res2 != nil {
		t.Errorf("expected nil for unrelated tool, got %v", res2)
	}
}

func TestInvalidPattern(t *testing.T) {
	_, err := router.New([]config.ServerConfig{
		{Name: "bad", Routing: &config.RoutingConfig{Pattern: `[invalid`}},
	})
	if err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestRoundRobin(t *testing.T) {
	r := makeRouter(t, nil)
	servers := []string{"a", "b", "c"}

	seen := map[string]int{}
	for i := 0; i < 30; i++ {
		s := r.RoundRobin(servers, nil)
		seen[s]++
	}

	for _, name := range servers {
		if seen[name] != 10 {
			t.Errorf("server %q got %d requests, want 10", name, seen[name])
		}
	}
}

func TestWeightedRoundRobin(t *testing.T) {
	r := makeRouter(t, nil)
	servers := []string{"heavy", "light"}
	weights := []int{3, 1}

	seen := map[string]int{}
	for i := 0; i < 40; i++ {
		s := r.RoundRobin(servers, weights)
		seen[s]++
	}

	// heavy should get 3x light
	if seen["heavy"] != 30 {
		t.Errorf("heavy: got %d, want 30", seen["heavy"])
	}
	if seen["light"] != 10 {
		t.Errorf("light: got %d, want 10", seen["light"])
	}
}
