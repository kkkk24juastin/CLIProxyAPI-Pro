package pool

import (
	"errors"
	"testing"
	"time"

	pluginconfig "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/proxy-pool/internal/config"
)

func testConfig(strategy string) pluginconfig.Config {
	return pluginconfig.Config{
		Strategy: strategy,
		Nodes: []pluginconfig.NodeConfig{
			{ID: "a", URL: "http://user:secret@127.0.0.1:19001", Enabled: true, Weight: 2, Order: 10},
			{ID: "b", URL: "http://127.0.0.1:19002", Enabled: true, Weight: 1, Order: 20},
		},
	}
}

func TestRoundRobinStartsAtFirstNode(t *testing.T) {
	p := New(testConfig("round-robin"))
	want := []string{"a", "b", "a", "b"}
	for index, expected := range want {
		selected := p.Select(nil)
		if selected == nil || selected.ID() != expected {
			t.Fatalf("Select() #%d = %#v, want %s", index, selected, expected)
		}
	}
}

func TestWeightedUsesSmoothDistribution(t *testing.T) {
	p := New(testConfig("weighted"))
	counts := map[string]int{}
	for range 6 {
		counts[p.Select(nil).ID()]++
	}
	if counts["a"] != 4 || counts["b"] != 2 {
		t.Fatalf("weighted counts = %+v, want a:4 b:2", counts)
	}
}

func TestIsolationRemovesNodeUntilExpiry(t *testing.T) {
	p := New(testConfig("round-robin"))
	node := p.Node("a")
	node.MarkAttempt()
	node.MarkFailure(errors.New("dial failed"), 1, 50*time.Millisecond)
	if selected := p.Select(nil); selected == nil || selected.ID() != "b" {
		t.Fatalf("Select() while a isolated = %#v, want b", selected)
	}
	time.Sleep(60 * time.Millisecond)
	foundA := false
	for range 2 {
		if selected := p.Select(nil); selected != nil && selected.ID() == "a" {
			foundA = true
		}
	}
	if !foundA {
		t.Fatal("expired isolated node did not become eligible")
	}
}

func TestRedactURL(t *testing.T) {
	got := RedactURL("socks5://alice:secret@proxy.example:1080")
	if got != "socks5://alice:***@proxy.example:1080" {
		t.Fatalf("RedactURL() = %q", got)
	}
}
