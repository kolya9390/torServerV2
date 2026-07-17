package cliapp

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
)

type contextView struct {
	Name            string `json:"name"`
	Server          string `json:"server"`
	User            string `json:"user,omitempty"`
	Current         bool   `json:"current"`
	Insecure        bool   `json:"insecure"`
	TokenConfigured bool   `json:"token_configured"`
}

func contextList(cfg *contextConfig, opts globalOptions) error {
	if cfg == nil {
		return errors.New("context config is nil")
	}

	contexts := make([]contextView, 0, len(cfg.Contexts))

	for _, name := range cfg.contextNames() {
		ctx := cfg.Contexts[name]
		contexts = append(contexts, contextView{
			Name:            name,
			Server:          redactURLCredentials(ctx.Server),
			User:            ctx.User,
			Current:         cfg.Current == name,
			Insecure:        ctx.Insecure,
			TokenConfigured: strings.TrimSpace(ctx.Token) != "",
		})
	}

	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), contexts)
	}

	table := tabwriter.NewWriter(opts.stdoutWriter(), 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "CURRENT\tNAME\tSERVER\tUSER\tINSECURE\tTOKEN")

	for _, ctx := range contexts {
		current := ""
		if ctx.Current {
			current = "*"
		}

		tokenSet := "no"
		if ctx.TokenConfigured {
			tokenSet = "yes"
		}

		_, _ = fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%v\t%s\n",
			current,
			ctx.Name,
			ctx.Server,
			ctx.User,
			ctx.Insecure,
			tokenSet,
		)
	}

	return table.Flush()
}

func contextCurrent(cfg *contextConfig, opts globalOptions) error {
	if cfg == nil {
		return errors.New("context config is nil")
	}

	ctx, ok := cfg.Contexts[cfg.Current]

	if !ok {
		return fmt.Errorf("current context %q is not configured", cfg.Current)
	}

	view := contextView{
		Name:            cfg.Current,
		Server:          redactURLCredentials(ctx.Server),
		User:            ctx.User,
		Current:         true,
		Insecure:        ctx.Insecure,
		TokenConfigured: strings.TrimSpace(ctx.Token) != "",
	}
	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), view)
	}

	lines := []string{
		"Current context: " + cfg.Current,
		"Server: " + view.Server,
	}
	if strings.TrimSpace(ctx.User) != "" {
		lines = append(lines, "User: "+ctx.User)
	}

	lines = append(
		lines,
		fmt.Sprintf("Insecure TLS: %v", ctx.Insecure),
		fmt.Sprintf("Token configured: %v", view.TokenConfigured),
	)

	return writeTextLines(opts.stdoutWriter(), lines...)
}

func contextAdd(cfg *contextConfig, opts globalOptions, name string, entry contextEntry) error {
	ctxName := normalizeContextName(name)

	if ctxName == "" {
		return errors.New("context add requires --name")
	}

	serverURL := strings.TrimSpace(entry.Server)

	if serverURL == "" {
		return errors.New("context add requires --server")
	}

	cfg.Contexts[ctxName] = contextEntry{
		Server:   serverURL,
		User:     strings.TrimSpace(entry.User),
		Pass:     strings.TrimSpace(entry.Pass),
		Token:    strings.TrimSpace(entry.Token),
		Insecure: entry.Insecure,
	}

	if strings.TrimSpace(cfg.Current) == "" {
		cfg.Current = ctxName
	}

	if err := opts.saveContexts(cfg); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "context_saved", "name": ctxName},
		fmt.Sprintf("OK: context %q saved", ctxName),
	)
}

func contextUse(cfg *contextConfig, opts globalOptions, name string) error {
	ctxName := normalizeContextName(name)

	if ctxName == "" {
		return errors.New("context use requires --name")
	}

	if _, ok := cfg.Contexts[ctxName]; !ok {
		return fmt.Errorf("context %q not found", ctxName)
	}

	cfg.Current = ctxName

	if err := opts.saveContexts(cfg); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "context_selected", "name": ctxName},
		"OK: current context -> "+ctxName,
	)
}

func contextRemove(cfg *contextConfig, opts globalOptions, name string) error {
	ctxName := normalizeContextName(name)

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

	if err := opts.saveContexts(cfg); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "context_removed", "name": ctxName},
		fmt.Sprintf("OK: context %q removed", ctxName),
	)
}

func normalizeContextName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
