package cliapp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var errOperationCanceled = errors.New("operation canceled")

type confirmationRequest struct {
	Action      string
	Yes         bool
	Interactive bool
	Input       io.Reader
	Output      io.Writer
}

func confirmDestructiveAction(ctx context.Context, request confirmationRequest) error {
	if request.Yes {
		return nil
	}

	if !request.Interactive {
		return fmt.Errorf("%s requires confirmation; rerun with --yes in non-interactive mode", request.Action)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("confirm %s: %w", request.Action, err)
	}

	if _, err := fmt.Fprintf(request.Output, "%s. Type 'yes' to continue: ", request.Action); err != nil {
		return fmt.Errorf("write confirmation prompt: %w", err)
	}

	scanner := bufio.NewScanner(request.Input)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}

		return errOperationCanceled
	}

	if !strings.EqualFold(strings.TrimSpace(scanner.Text()), "yes") {
		return errOperationCanceled
	}

	return nil
}

func confirmCommand(cmd *cobra.Command, opts *globalOptions, action string, yes bool) error {
	input := cmd.InOrStdin()
	interactive := readerIsTerminal(input)
	if opts != nil && opts.runtime != nil {
		interactive = opts.runtime.isTerminal(input)
	}

	return confirmDestructiveAction(cmd.Context(), confirmationRequest{
		Action:      action,
		Yes:         yes,
		Interactive: interactive,
		Input:       input,
		Output:      cmd.ErrOrStderr(),
	})
}

func readerIsTerminal(reader io.Reader) bool {
	fd, ok := readerFileDescriptor(reader)

	return ok && term.IsTerminal(fd)
}

func readerFileDescriptor(reader io.Reader) (int, bool) {
	file, ok := reader.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}

	return int(file.Fd()), true
}
