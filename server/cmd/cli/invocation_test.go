package cli

import "testing"

func TestIsInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no arguments starts server", want: false},
		{name: "direct command", args: []string{"status"}, want: true},
		{name: "unknown command remains cli error", args: []string{"unknown"}, want: true},
		{name: "server flag", args: []string{"--port", "8090"}, want: false},
		{name: "server boolean flag", args: []string{"--httpauth"}, want: false},
		{name: "server flag value resembles command", args: []string{"--path", "status"}, want: false},
		{name: "cli value flag before command", args: []string{"--server", "http://localhost:8090", "status"}, want: true},
		{name: "cli equals flag before command", args: []string{"--output=json", "torrents", "list"}, want: true},
		{name: "multiple cli flags", args: []string{"--context", "home", "--timeout", "5s", "url", "1"}, want: true},
		{name: "cli boolean flag", args: []string{"--insecure", "status"}, want: true},
		{name: "standard version flag", args: []string{"--version"}, want: true},
		{name: "missing cli flag value", args: []string{"--server"}, want: true},
		{name: "cli flags without command", args: []string{"--server", "http://localhost:8090"}, want: true},
		{name: "unknown leading flag stays server", args: []string{"--unknown", "status"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := IsInvocation(test.args); got != test.want {
				t.Fatalf("IsInvocation(%q) = %t, want %t", test.args, got, test.want)
			}
		})
	}
}
