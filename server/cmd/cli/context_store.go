package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type contextConfig struct {
	Current  string                  `json:"current"`
	Contexts map[string]contextEntry `json:"contexts"`
}

type contextEntry struct {
	Server   string `json:"server"`
	User     string `json:"user,omitempty"`
	Pass     string `json:"pass,omitempty"`
	Token    string `json:"token,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`
}

func loadContextConfig() (*contextConfig, error) {
	cfgPath, err := contextConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultContextConfig(), nil
		}

		return nil, fmt.Errorf("read context config: %w", err)
	}

	var cfg contextConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse context config: %w", err)
	}

	normalizeContextConfig(&cfg)

	return &cfg, nil
}

func saveContextConfig(cfg *contextConfig) error {
	if cfg == nil {
		return errors.New("nil context config")
	}

	normalizeContextConfig(cfg)

	cfgPath, err := contextConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode context config: %w", err)
	}

	if err := os.WriteFile(cfgPath, payload, 0o600); err != nil {
		return fmt.Errorf("write context config: %w", err)
	}

	return nil
}

func contextConfigPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("TSCTL_CONFIG")); custom != "" {
		return custom, nil
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(cfgDir, "tsctl", "config.json"), nil
}

func defaultContextConfig() *contextConfig {
	return &contextConfig{
		Current: "local",
		Contexts: map[string]contextEntry{
			"local": {
				Server: "http://127.0.0.1:8090",
			},
		},
	}
}

func normalizeContextConfig(cfg *contextConfig) {
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]contextEntry{}
	}

	if _, ok := cfg.Contexts["local"]; !ok {
		cfg.Contexts["local"] = contextEntry{
			Server: "http://127.0.0.1:8090",
		}
	}

	if strings.TrimSpace(cfg.Current) == "" {
		cfg.Current = "local"
	}

	if _, ok := cfg.Contexts[cfg.Current]; !ok {
		cfg.Current = "local"
	}
}

func (cfg *contextConfig) contextNames() []string {
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func applyContextToOptions(opts globalOptions) (globalOptions, error) {
	cfg, err := loadContextConfig()
	if err != nil {
		return globalOptions{}, err
	}

	ctxName := strings.TrimSpace(opts.Context)
	if ctxName == "" {
		ctxName = cfg.Current
	}

	entry, ok := cfg.Contexts[ctxName]
	if !ok {
		return globalOptions{}, fmt.Errorf("unknown context: %s", ctxName)
	}

	out := opts
	out.Context = ctxName

	if strings.TrimSpace(out.Server) == "" {
		out.Server = entry.Server
	}

	if strings.TrimSpace(out.User) == "" {
		out.User = entry.User
	}

	if strings.TrimSpace(out.Pass) == "" {
		out.Pass = entry.Pass
	}

	if strings.TrimSpace(out.Token) == "" {
		out.Token = entry.Token
	}

	if !out.insecureExplicit {
		out.Insecure = entry.Insecure
	}

	if strings.TrimSpace(out.Server) == "" {
		out.Server = "http://127.0.0.1:8090"
	}

	return out, nil
}
