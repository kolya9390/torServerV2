package cliapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type contextStore interface {
	Load() (*contextConfig, error)
	Save(*contextConfig) error
}

type fileContextStore struct {
	fileSystem FileSystem
	getenv     func(string) string
}

func newFileContextStore(fileSystem FileSystem, getenv func(string) string) contextStore {
	return &fileContextStore{fileSystem: fileSystem, getenv: getenv}
}

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

func (store *fileContextStore) Load() (*contextConfig, error) {
	cfgPath, err := store.configPath()
	if err != nil {
		return nil, err
	}

	data, err := store.fileSystem.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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

func (store *fileContextStore) Save(cfg *contextConfig) error {
	if cfg == nil {
		return errors.New("nil context config")
	}

	normalizeContextConfig(cfg)

	cfgPath, err := store.configPath()
	if err != nil {
		return err
	}

	if err := store.fileSystem.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode context config: %w", err)
	}

	if err := store.fileSystem.WriteFile(cfgPath, payload, 0o600); err != nil {
		return fmt.Errorf("write context config: %w", err)
	}

	return nil
}

func (store *fileContextStore) configPath() (string, error) {
	if custom := strings.TrimSpace(store.getenv(envConfig)); custom != "" {
		return custom, nil
	}

	cfgDir, err := store.fileSystem.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(cfgDir, "tsctl", "config.json"), nil
}

func defaultContextConfig() *contextConfig {
	return &contextConfig{
		Current: defaultContextName,
		Contexts: map[string]contextEntry{
			defaultContextName: {
				Server: defaultServerURL,
			},
		},
	}
}

func normalizeContextConfig(cfg *contextConfig) {
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]contextEntry{}
	}

	if _, ok := cfg.Contexts[defaultContextName]; !ok {
		cfg.Contexts[defaultContextName] = contextEntry{
			Server: defaultServerURL,
		}
	}

	if strings.TrimSpace(cfg.Current) == "" {
		cfg.Current = defaultContextName
	}

	if _, ok := cfg.Contexts[cfg.Current]; !ok {
		cfg.Current = defaultContextName
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
	if opts.runtime == nil || opts.runtime.contextStore == nil {
		return globalOptions{}, errors.New("context store is not initialized")
	}

	cfg, err := opts.runtime.contextStore.Load()
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
		out.Server = defaultServerURL
	}

	return out, nil
}
