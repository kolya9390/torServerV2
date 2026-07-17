package daemon

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	buildversion "server/version"
)

func TestParseProcessArgsUsesProvidedArguments(t *testing.T) {
	t.Parallel()

	args, err := parseProcessArgs([]string{"--port", "18090", "--ssl"}, &bytes.Buffer{}, testBuildInfo())
	if err != nil {
		t.Fatalf("parse arguments: %v", err)
	}

	if args.Port != "18090" || !args.Ssl {
		t.Fatalf("parsed arguments = %+v", args)
	}
}

func TestParseProcessArgsAcceptsCanonicalServeCommand(t *testing.T) {
	t.Parallel()

	args, err := parseProcessArgs([]string{"serve", "--port", "18090"}, &bytes.Buffer{}, testBuildInfo())
	if err != nil {
		t.Fatalf("parse serve arguments: %v", err)
	}

	if args.Port != "18090" {
		t.Fatalf("port = %q, want 18090", args.Port)
	}
}

func TestParseProcessArgsPreservesBareDaemonAlias(t *testing.T) {
	t.Parallel()

	args, err := parseProcessArgs(nil, &bytes.Buffer{}, testBuildInfo())
	if err != nil {
		t.Fatalf("parse bare invocation: %v", err)
	}

	if args == nil {
		t.Fatal("bare invocation returned nil arguments")
	}
}

func TestParseProcessArgsHelpDoesNotExitProcess(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	args, err := parseProcessArgs([]string{"serve", "--help"}, &output, testBuildInfo())
	if args != nil || !errors.Is(err, errHelpRequested) {
		t.Fatalf("help result = (%+v, %v)", args, err)
	}

	if !strings.Contains(output.String(), "Usage: torrserver serve") {
		t.Fatalf("help output does not document canonical invocation:\n%s", output.String())
	}
}

func TestParseProcessArgsVersionUsesCanonicalBuildMetadata(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	args, err := parseProcessArgs([]string{"serve", "--version"}, &output, testBuildInfo())
	if args != nil || !errors.Is(err, errVersionRequested) {
		t.Fatalf("version result = (%+v, %v)", args, err)
	}

	const expected = "torrserver v1.0.0-beta.test (darwin/arm64, commit 0123456789ab)\n"
	if output.String() != expected {
		t.Fatalf("version output = %q, want %q", output.String(), expected)
	}
}

func TestParseProcessArgsRejectsUnknownPositionalArgument(t *testing.T) {
	t.Parallel()

	args, err := parseProcessArgs([]string{"unknown"}, &bytes.Buffer{}, testBuildInfo())
	if args != nil || err == nil {
		t.Fatalf("unknown argument result = (%+v, %v)", args, err)
	}
}

func TestParseProcessArgsInvocationContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantPort   string
		wantPath   string
		wantSignal error
		wantUsage  bool
	}{
		{name: "bare daemon compatibility"},
		{name: "canonical serve", args: []string{"serve"}},
		{name: "bare server flag", args: []string{"--port", "18090"}, wantPort: "18090"},
		{
			name:     "canonical combined server flags",
			args:     []string{"serve", "--port", "18091", "--path", "./data", "--ssl"},
			wantPort: "18091",
			wantPath: "./data",
		},
		{name: "server value resembles command", args: []string{"--path", "status"}, wantPath: "status"},
		{name: "bare help", args: []string{"--help"}, wantSignal: errHelpRequested},
		{name: "canonical help", args: []string{"serve", "--help"}, wantSignal: errHelpRequested},
		{name: "bare version", args: []string{"--version"}, wantSignal: errVersionRequested},
		{name: "canonical version", args: []string{"serve", "--version"}, wantSignal: errVersionRequested},
		{name: "unknown command", args: []string{"unknown"}, wantUsage: true},
		{name: "unknown flag", args: []string{"--unknown=private-value"}, wantUsage: true},
		{name: "command after explicit serve", args: []string{"serve", "shutdown"}, wantUsage: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			args, err := parseProcessArgs(test.args, &output, testBuildInfo())
			if test.wantSignal != nil {
				if args != nil || !errors.Is(err, test.wantSignal) {
					t.Fatalf("informational result = (%+v, %v)", args, err)
				}

				return
			}

			if test.wantUsage {
				var usageErr *invalidArgumentsError
				if args != nil || !errors.As(err, &usageErr) {
					t.Fatalf("usage result = (%+v, %v)", args, err)
				}

				return
			}

			if err != nil || args == nil {
				t.Fatalf("daemon result = (%+v, %v)", args, err)
			}

			if args.Port != test.wantPort || args.Path != test.wantPath {
				t.Fatalf("parsed args = %+v, want port=%q path=%q", args, test.wantPort, test.wantPath)
			}
		})
	}
}

func TestParseProcessArgsMigratesFormerManagementCommands(t *testing.T) {
	t.Parallel()

	commands := []string{
		"auth",
		"completion",
		"config",
		"context",
		"settings",
		"shutdown",
		"status",
		"torrents",
		"url",
	}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			args, err := parseProcessArgs([]string{command, "ignored-subcommand"}, &bytes.Buffer{}, testBuildInfo())
			var migrationErr *managementCommandError
			if args != nil || !errors.As(err, &migrationErr) {
				t.Fatalf("migration result = (%+v, %v)", args, err)
			}

			if migrationErr.Command != command || migrationErr.Replacement != "torrctl "+command {
				t.Fatalf("migration error = %+v", migrationErr)
			}
		})
	}
}

func TestParseProcessArgsMigrationDoesNotExposeLegacyFlagValues(t *testing.T) {
	t.Parallel()

	const (
		secret = "do-not-print-this-token"
		host   = "https://user:password@example.test/private"
	)

	args, err := parseProcessArgs([]string{
		"--server", host,
		"--pass=" + secret,
		"--token", secret,
		"--output=json",
		"shutdown",
	}, &bytes.Buffer{}, testBuildInfo())
	var migrationErr *managementCommandError
	if args != nil || !errors.As(err, &migrationErr) {
		t.Fatalf("migration result = (%+v, %v)", args, err)
	}

	message := UserMessage(err)
	if message != "torrserver shutdown: management commands moved to torrctl; use `torrctl shutdown`" {
		t.Fatalf("migration message = %q", message)
	}

	if strings.Contains(message, secret) || strings.Contains(message, host) {
		t.Fatalf("migration message exposes private value: %q", message)
	}
}

func TestParseProcessArgsInvalidInputUsesRedactedUsageMessage(t *testing.T) {
	t.Parallel()

	const privateValue = "/Users/private/config.yml"

	args, err := parseProcessArgs([]string{"--unknown=" + privateValue}, &bytes.Buffer{}, testBuildInfo())
	var usageErr *invalidArgumentsError
	if args != nil || !errors.As(err, &usageErr) {
		t.Fatalf("usage result = (%+v, %v)", args, err)
	}

	message := UserMessage(err)
	if strings.Contains(message, privateValue) || !strings.Contains(message, "usage: torrserver serve [flags]") {
		t.Fatalf("unsafe or incomplete usage message = %q", message)
	}
}

func testBuildInfo() buildversion.Info {
	return buildversion.Info{
		Version: "v1.0.0-beta.test",
		Commit:  "0123456789abcdef",
		OS:      "darwin",
		Arch:    "arm64",
	}
}
