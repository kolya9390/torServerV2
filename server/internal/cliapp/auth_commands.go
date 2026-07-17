package cliapp

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"golang.org/x/term"
)

// cmdAuthList lists all users on the server.
func cmdAuthList(cli authAPI, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	users, err := cli.ListUsers(ctx)
	if err != nil {
		return err
	}

	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), users)
	}

	if len(users) == 0 {
		_, err := fmt.Fprintln(opts.stdoutWriter(), "No users found")

		return err
	}

	w := tabwriter.NewWriter(opts.stdoutWriter(), 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "USERNAME\tCREATED_AT")

	for name, createdAt := range users {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", name, createdAt)
	}

	return w.Flush()
}

// cmdAuthAdd creates a new user on the server.
func cmdAuthAdd(cli authAPI, opts globalOptions, username, password string) error {
	if username == "" {
		return errors.New("username is required")
	}

	// If password is not provided, prompt for it
	if password == "" {
		pass, err := opts.promptNewPassword(opts.stderrWriter())
		if err != nil {
			return err
		}

		password = pass
	}

	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	if err := cli.AddUser(ctx, username, password); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "user_created", "username": username},
		fmt.Sprintf("OK: user %q created", username),
	)
}

// cmdAuthRemove removes a user from the server.
func cmdAuthRemove(cli authAPI, opts globalOptions, username string) error {
	if username == "" {
		return errors.New("username is required")
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	if err := cli.RemoveUser(ctx, username); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "user_removed", "username": username},
		fmt.Sprintf("OK: user %q removed", username),
	)
}

// readPasswordInteractively prompts the user for a password without echoing input.
func readPasswordInteractively(input io.Reader, output io.Writer) (string, error) {
	fd, ok := readerFileDescriptor(input)
	if !ok {
		return "", errors.New("password input is not a terminal; use --password in non-interactive mode")
	}

	_, _ = fmt.Fprint(output, "Enter new password: ")

	pass, err := term.ReadPassword(fd)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	_, _ = fmt.Fprintln(output)
	_, _ = fmt.Fprint(output, "Confirm password: ")

	confirm, err := term.ReadPassword(fd)
	if err != nil {
		return "", fmt.Errorf("read confirmation: %w", err)
	}

	_, _ = fmt.Fprintln(output)

	if string(pass) != string(confirm) {
		return "", errors.New("passwords do not match")
	}

	return string(pass), nil
}
