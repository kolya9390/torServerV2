package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"golang.org/x/term"
)

// cmdAuthList lists all users on the server.
func cmdAuthList(cli *apiClient, opts globalOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	var users map[string]string
	if err := cli.doJSON(ctx, http.MethodGet, "/api/v1/auth/users", nil, &users, nil); err != nil {
		return err
	}

	if len(users) == 0 {
		fmt.Println("No users found")

		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
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
		pass, err := readPasswordInteractively()
		if err != nil {
			return err
		}

		password = pass
	}

	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{
		"username": username,
		"password": password,
	}

	var resp map[string]any
	if err := cli.doJSON(ctx, http.MethodPost, "/api/v1/auth/users", payload, &resp, nil); err != nil {
		return err
	}

	fmt.Printf("OK: user '%s' created\n", username)

	return nil
}

// cmdAuthRemove removes a user from the server.
func cmdAuthRemove(cli *apiClient, opts globalOptions, username string) error {
	if username == "" {
		return errors.New("username is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	url := "/api/v1/auth/users/" + username
	if err := cli.doJSON(ctx, http.MethodDelete, url, nil, nil, nil); err != nil {
		return err
	}

	fmt.Printf("OK: user '%s' removed\n", username)

	return nil
}

// readPasswordInteractively prompts the user for a password without echoing input.
func readPasswordInteractively() (string, error) {
	fmt.Print("Enter new password: ")

	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	fmt.Println()
	fmt.Print("Confirm password: ")

	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read confirmation: %w", err)
	}

	fmt.Println()

	if string(pass) != string(confirm) {
		return "", errors.New("passwords do not match")
	}

	return string(pass), nil
}
