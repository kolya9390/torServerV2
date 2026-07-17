package cliapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestResolveClientOptionsPrecedence(t *testing.T) {
	configPath := writeContextConfigFixture(t, contextConfig{
		Current: "home",
		Contexts: map[string]contextEntry{
			"home": {
				Server:   "http://context.example:8090",
				User:     "context-user",
				Pass:     "context-pass",
				Token:    "context-token",
				Insecure: true,
			},
		},
	})
	t.Setenv(envConfig, configPath)
	t.Setenv(envUser, "env-user")
	t.Setenv(envPassword, "env-pass")
	t.Setenv(envToken, "env-token")

	tests := []struct {
		name string
		opts globalOptions
		want globalOptions
	}{
		{
			name: "environment overrides context",
			opts: globalOptions{Timeout: time.Second, Output: outputTable},
			want: globalOptions{
				Context:  "home",
				Server:   "http://context.example:8090",
				User:     "env-user",
				Pass:     "env-pass",
				Token:    "env-token",
				Insecure: true,
			},
		},
		{
			name: "explicit values override environment",
			opts: globalOptions{
				Server:  "https://flag.example:8091",
				User:    "flag-user",
				Pass:    "flag-pass",
				Token:   "flag-token",
				Timeout: time.Second,
				Output:  outputJSON,
			},
			want: globalOptions{
				Context:  "home",
				Server:   "https://flag.example:8091",
				User:     "flag-user",
				Pass:     "flag-pass",
				Token:    "flag-token",
				Insecure: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveClientOptions(nil, test.opts)
			if err != nil {
				t.Fatalf("resolve options: %v", err)
			}

			assertResolvedOptions(t, got, test.want)
		})
	}
}

func TestResolveClientOptionsExplicitSecureTLSOverridesContext(t *testing.T) {
	configPath := writeContextConfigFixture(t, contextConfig{
		Current: "home",
		Contexts: map[string]contextEntry{
			"home": {Server: "https://context.example:8091", Insecure: true},
		},
	})
	t.Setenv(envConfig, configPath)

	cmd := &cobra.Command{}
	cmd.Flags().Bool("insecure", false, "")

	if err := cmd.Flags().Set("insecure", "false"); err != nil {
		t.Fatalf("set explicit insecure flag: %v", err)
	}

	got, err := resolveClientOptions(cmd, globalOptions{Timeout: time.Second, Output: outputTable})
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}

	if got.Insecure {
		t.Fatal("explicit --insecure=false must override insecure context")
	}
}

func TestResolveClientOptionsLoadsContextCredentialsBeforePromptDecision(t *testing.T) {
	configPath := writeContextConfigFixture(t, contextConfig{
		Current: "home",
		Contexts: map[string]contextEntry{
			"home": {
				Server: "http://context.example:8090",
				User:   "context-user",
				Pass:   "context-pass",
			},
		},
	})
	t.Setenv(envConfig, configPath)
	t.Setenv(envUser, "")
	t.Setenv(envPassword, "")
	t.Setenv(envToken, "")

	got, err := resolveClientOptions(nil, globalOptions{Timeout: time.Second, Output: outputTable})
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}

	if got.User != "context-user" || got.Pass != "context-pass" {
		t.Fatalf("context credentials = (%q, %q), want configured pair", got.User, got.Pass)
	}
}

func TestResolveClientPasswordNonInteractiveFailsWithoutPrompt(t *testing.T) {
	promptCalled := false
	readPassword := func() (string, error) {
		promptCalled = true

		return "unexpected", nil
	}

	_, err := resolveClientPassword(globalOptions{User: "admin"}, false, readPassword)
	if err == nil || err.Error() != "password is required for the selected user; set TS_PASSWORD or configure the context password" {
		t.Fatalf("error = %v, want non-interactive password guidance", err)
	}

	if promptCalled {
		t.Fatal("non-interactive invocation must not prompt for a password")
	}
}

func TestResolveClientPasswordInteractiveUsesPrompt(t *testing.T) {
	want := globalOptions{User: "admin"}

	got, err := resolveClientPassword(want, true, func() (string, error) {
		return "secret", nil
	})
	if err != nil {
		t.Fatalf("resolve password: %v", err)
	}

	if got.User != "admin" || got.Pass != "secret" {
		t.Fatalf("credentials = (%q, %q), want prompted password", got.User, got.Pass)
	}
}

func writeContextConfigFixture(t *testing.T, cfg contextConfig) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "contexts.json")
	payload, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal context fixture: %v", err)
	}

	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write context fixture: %v", err)
	}

	return path
}

func assertResolvedOptions(t *testing.T, got, want globalOptions) {
	t.Helper()

	if got.Context != want.Context || got.Server != want.Server || got.User != want.User || got.Pass != want.Pass ||
		got.Token != want.Token || got.Insecure != want.Insecure {
		t.Fatalf("resolved options = %#v, want %#v", got, want)
	}
}
