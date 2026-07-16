package cli

import "strings"

var persistentValueFlags = map[string]struct{}{
	"--context": {},
	"--output":  {},
	"--pass":    {},
	"--server":  {},
	"--timeout": {},
	"--token":   {},
	"--user":    {},
}

// IsInvocation reports whether args belong to the management CLI rather than
// the server process. Server flags are deliberately not parsed here: an
// unrecognized leading flag keeps the invocation in server mode.
func IsInvocation(args []string) bool {
	cliFlagSeen := false

	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if arg == "--insecure" || arg == "--version" {
			cliFlagSeen = true

			continue
		}

		name, hasValue := splitFlag(arg)
		if _, ok := persistentValueFlags[name]; ok {
			cliFlagSeen = true

			if !hasValue && idx+1 < len(args) {
				idx++
			}

			continue
		}

		if strings.HasPrefix(arg, "-") {
			return false
		}

		return true
	}

	return cliFlagSeen
}

func splitFlag(arg string) (string, bool) {
	name, _, found := strings.Cut(arg, "=")

	return name, found
}
