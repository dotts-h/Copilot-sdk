package copilot

import (
	"testing"

	sdk "github.com/github/copilot-sdk/go"
)

func TestToAgentMode(t *testing.T) {
	cases := []struct {
		in   string
		want sdk.AgentMode
	}{
		{"plan", sdk.AgentModePlan},
		{"autopilot", sdk.AgentModeAutopilot},
		{"interactive", sdk.AgentModeInteractive},
		{"shell", sdk.AgentModeShell},
		{"", ""},
		{"bogus", ""}, // unknown maps to the zero mode (runtime keeps current)
	}
	for _, c := range cases {
		if got := toAgentMode(c.in); got != c.want {
			t.Errorf("toAgentMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
