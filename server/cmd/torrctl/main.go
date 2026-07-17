package main

import (
	"context"
	"os"

	"server/internal/cliapp"
)

func main() {
	dependencies := cliapp.DefaultDependencies()
	dependencies.ProgramName = "torrctl"

	os.Exit(cliapp.Run(cliapp.Invocation{
		Context: context.Background(),
		Args:    os.Args[1:],
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}, dependencies))
}
