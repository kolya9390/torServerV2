package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"golang.org/x/term"
)

// cmdAuthList lists all users on the server.
func cmdAuthList(cli *apiClient, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	var users map[string]string
	if err := cli.doJSON(ctx, http.MethodGet, "/api/v1/auth/users", nil, &users, nil); err != nil {
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
func cmdAuthAdd(cli *apiClient, opts globalOptions, username, password string) error {
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

	payload := map[string]any{
		"username": username,
		"password": password,
	}

	var resp map[string]any
	if err := cli.doJSON(ctx, http.MethodPost, "/api/v1/auth/users", payload, &resp, nil); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "user_created", "username": username},
		fmt.Sprintf("OK: user %q created", username),
	)
}

// cmdAuthRemove removes a user from the server.
func cmdAuthRemove(cli *apiClient, opts globalOptions, username string) error {
	if username == "" {
		return errors.New("username is required")
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	url := "/api/v1/auth/users/" + username
	if err := cli.doJSON(ctx, http.MethodDelete, url, nil, nil, nil); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "user_removed", "username": username},
		fmt.Sprintf("OK: user %q removed", username),
	)
}

// readPasswordInteractively prompts the user for a password without echoing input.
func readPasswordInteractively(output io.Writer) (string, error) {
	_, _ = fmt.Fprint(output, "Enter new password: ")

	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	_, _ = fmt.Fprintln(output)
	_, _ = fmt.Fprint(output, "Confirm password: ")

	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read confirmation: %w", err)
	}

	_, _ = fmt.Fprintln(output)

	if string(pass) != string(confirm) {
		return "", errors.New("passwords do not match")
	}

	return string(pass), nil
}
