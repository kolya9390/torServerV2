package main

import (
	"fmt"
	"os"

	"server/internal/releasebundle"
)

const usage = "usage: releasebundle create <repository-root> <release-dir> <version> | " +
	"verify <release-dir> <version> <commit>"

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}

	switch args[0] {
	case "create":
		if len(args) != 4 {
			return fmt.Errorf("%s", usage)
		}

		return releasebundle.CreateAll(args[1], args[2], args[3])
	case "verify":
		if len(args) != 4 {
			return fmt.Errorf("%s", usage)
		}

		return releasebundle.VerifyAll(args[1], args[2], args[3])
	default:
		return fmt.Errorf("unknown releasebundle operation %q; %s", args[0], usage)
	}
}
