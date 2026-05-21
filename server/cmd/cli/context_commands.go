package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func contextList(cfg *contextConfig) error {
	if cfg == nil {
		return errors.New("context config is nil")
	}

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "CURRENT\tNAME\tSERVER\tUSER\tINSECURE\tTOKEN")

	for _, name := range cfg.contextNames() {
		ctx := cfg.Contexts[name]

		current := ""

		if cfg.Current == name {
			current = "*"
		}

		tokenSet := "no"

		if strings.TrimSpace(ctx.Token) != "" {
			tokenSet = "yes"
		}

		_, _ = fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%v\t%s\n",
			current,
			name,
			ctx.Server,
			ctx.User,
			ctx.Insecure,
			tokenSet,
		)
	}

	return w.Flush()
}

func contextCurrent(cfg *contextConfig) error {
	if cfg == nil {
		return errors.New("context config is nil")
	}

	ctx, ok := cfg.Contexts[cfg.Current]

	if !ok {
		return fmt.Errorf("current context %q is not configured", cfg.Current)
	}

	fmt.Printf("Current context: %s\n", cfg.Current)
	fmt.Printf("Server: %s\n", ctx.Server)

	if strings.TrimSpace(ctx.User) != "" {
		fmt.Printf("User: %s\n", ctx.User)
	}

	fmt.Printf("Insecure TLS: %v\n", ctx.Insecure)
	fmt.Printf("Token configured: %v\n", strings.TrimSpace(ctx.Token) != "")

	return nil
}

func contextAdd(cfg *contextConfig, args []string) error {
	fs := flag.NewFlagSet("context add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	name := fs.String("name", "", "context name")
	server := fs.String("server", "", "server URL")
	user := fs.String("user", "", "basic auth user")
	pass := fs.String("pass", "", "basic auth password")
	token := fs.String("token", "", "shutdown token")
	insecure := fs.Bool("insecure", false, "skip TLS verification")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ctxName := normalizeContextName(*name)

	if ctxName == "" {
		return errors.New("context add requires --name")
	}

	serverURL := strings.TrimSpace(*server)

	if serverURL == "" {
		return errors.New("context add requires --server")
	}

	cfg.Contexts[ctxName] = contextEntry{
		Server:   serverURL,
		User:     strings.TrimSpace(*user),
		Pass:     strings.TrimSpace(*pass),
		Token:    strings.TrimSpace(*token),
		Insecure: *insecure,
	}

	if strings.TrimSpace(cfg.Current) == "" {
		cfg.Current = ctxName
	}

	if err := saveContextConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("OK: context %q saved\n", ctxName)

	return nil
}

func contextUse(cfg *contextConfig, args []string) error {
	fs := flag.NewFlagSet("context use", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	name := fs.String("name", "", "context name")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ctxName := normalizeContextName(*name)

	if ctxName == "" {
		return errors.New("context use requires --name")
	}

	if _, ok := cfg.Contexts[ctxName]; !ok {
		return fmt.Errorf("context %q not found", ctxName)
	}

	cfg.Current = ctxName

	if err := saveContextConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("OK: current context -> %s\n", ctxName)

	return nil
}

func contextRemove(cfg *contextConfig, args []string) error {
	fs := flag.NewFlagSet("context remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	name := fs.String("name", "", "context name")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ctxName := normalizeContextName(*name)

	if ctxName == "" {
		return errors.New("context remove requires --name")
	}

	if ctxName == "local" {
		return errors.New("local context cannot be removed")
	}

	if _, ok := cfg.Contexts[ctxName]; !ok {
		return fmt.Errorf("context %q not found", ctxName)
	}

	delete(cfg.Contexts, ctxName)

	if cfg.Current == ctxName {
		cfg.Current = "local"
	}

	if err := saveContextConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("OK: context %q removed\n", ctxName)

	return nil
}

func normalizeContextName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
