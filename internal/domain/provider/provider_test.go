package provider

import "testing"

func TestEnvVarDerivesFromProviderID(t *testing.T) {
	tests := map[ID]string{
		"opencode":    "OPENCODE_API_KEY",
		"anthropic":   "ANTHROPIC_API_KEY",
		"openai-main": "OPENAI_MAIN_API_KEY",
		"team2":       "TEAM2_API_KEY",
	}
	for id, want := range tests {
		if got := EnvVar(id); got != want {
			t.Errorf("EnvVar(%q) = %q, want %q", id, got, want)
		}
	}
}
