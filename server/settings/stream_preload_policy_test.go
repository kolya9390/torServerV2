package settings

import "testing"

func TestNormalizeStartupPreloadPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   string
	}{
		{name: "empty defaults to skip active", want: StartupPreloadPolicySkipActive},
		{name: "skip active", policy: StartupPreloadPolicySkipActive, want: StartupPreloadPolicySkipActive},
		{name: "legacy", policy: StartupPreloadPolicyLegacy, want: StartupPreloadPolicyLegacy},
		{name: "trims and lowercases", policy: " LEGACY ", want: StartupPreloadPolicyLegacy},
		{name: "unknown fails closed", policy: "always", want: StartupPreloadPolicySkipActive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeStartupPreloadPolicy(tt.policy); got != tt.want {
				t.Fatalf("NormalizeStartupPreloadPolicy(%q) = %q, want %q", tt.policy, got, tt.want)
			}
		})
	}
}
