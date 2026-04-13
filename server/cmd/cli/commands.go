package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"
)

type torrentStatus struct {
	Title         string  `json:"title"`
	Name          string  `json:"name"`
	Hash          string  `json:"hash"`
	Stat          int     `json:"stat"`
	StatString    string  `json:"stat_string"`
	TotalPeers    int     `json:"total_peers"`
	ActivePeers   int     `json:"active_peers"`
	PendingPeers  int     `json:"pending_peers"`
	DownloadSpeed float64 `json:"download_speed"`
	UploadSpeed   float64 `json:"upload_speed"`
}

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

func cmdStatus(cli *apiClient, opts globalOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	version := map[string]any{}

	if err := cli.doJSON(ctx, "GET", "/api/v1/version", nil, &version, nil); err != nil {
		return err
	}

	ready := map[string]any{}

	if err := cli.doJSON(ctx, "GET", "/readyz", nil, &ready, nil); err != nil {
		return err
	}

	if opts.Output == "json" {
		return printJSON(map[string]any{
			"server":  cli.baseURL.String(),
			"version": version,
			"ready":   ready,
		})
	}

	fmt.Printf("Server: %s\n", cli.baseURL.String())

	if strings.TrimSpace(opts.Context) != "" {
		fmt.Printf("Context: %s\n", opts.Context)
	}

	fmt.Printf("Version: %v\n", version["current"])
	fmt.Printf("Ready: %v\n", ready["status"])
	fmt.Printf("HTTP: %v\n", ready["http"])
	fmt.Printf("Torrent: %v\n", ready["torrent"])

	return nil
}

func cmdTorrentsList(cli *apiClient, opts globalOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{"action": "list"}

	var out []torrentStatus

	if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", payload, &out, nil); err != nil {
		return err
	}

	if opts.Output == "json" {
		return printJSON(out)
	}

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "#\tHASH\tSTATE\tPEERS\tDOWN\tUP\tTITLE")

	for i, t := range out {
		peers := fmt.Sprintf("%d/%d/%d", t.ActivePeers, t.PendingPeers, t.TotalPeers)
		_, _ = fmt.Fprintf(
			w,
			"%d\t%s\t%s\t%s\t%.2f\t%.2f\t%s\n",
			i+1,
			shortHash(t.Hash),
			t.StatString,
			peers,
			t.DownloadSpeed,
			t.UploadSpeed,
			firstNonEmpty(t.Title, t.Name),
		)
	}

	return w.Flush()
}

// resolveTorrentID fetches the torrent list and resolves the given identifier
// to a torrent hash. It accepts:
//   - Numeric index (1-based, from `torrents list`)
//   - Partial title/name (case-insensitive substring match)
//   - Full 40-char hex hash (direct passthrough)
func resolveTorrentID(cli *apiClient, timeout time.Duration, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)

	if identifier == "" {
		return "", errors.New("torrent identifier is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var torrents []torrentStatus

	if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", map[string]any{"action": "list"}, &torrents, nil); err != nil {
		return "", fmt.Errorf("fetch torrent list: %w", err)
	}

	// Try as numeric index (1-based)
	if idx, err := strconv.Atoi(identifier); err == nil {
		if idx < 1 || idx > len(torrents) {
			return "", fmt.Errorf("index %d out of range (1-%d)", idx, len(torrents))
		}

		return torrents[idx-1].Hash, nil
	}

	// Try as full hash (40 hex chars)
	if len(identifier) == 40 {
		for _, t := range torrents {
			if strings.EqualFold(t.Hash, identifier) {
				return t.Hash, nil
			}
		}
	}

	// Search by partial title/name match (case-insensitive)
	query := strings.ToLower(identifier)

	var matches []torrentStatus

	for _, t := range torrents {
		title := strings.ToLower(firstNonEmpty(t.Title, t.Name))

		if strings.Contains(title, query) {
			matches = append(matches, t)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no torrent found matching %q", identifier)
	}

	if len(matches) > 1 {
		// Show ambiguous matches
		var names []string

		for _, m := range matches {
			names = append(names, fmt.Sprintf("%s (%s)", firstNonEmpty(m.Title, m.Name), shortHash(m.Hash)))
		}

		return "", fmt.Errorf("ambiguous identifier %q matches multiple torrents:\n  - %s\nTry using the full hash or index number", identifier, strings.Join(names, "\n  - "))
	}

	return matches[0].Hash, nil
}

func cmdTorrentsGet(cli *apiClient, opts globalOptions, args []string) error {
	// Support positional argument or --hash for backward compatibility
	fs := flag.NewFlagSet("torrents get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	hash := fs.String("hash", "", "torrent hash, name, or index")

	if err := fs.Parse(args); err != nil {
		return err
	}

	identifier := strings.TrimSpace(*hash)

	if identifier == "" && len(fs.Args()) > 0 {
		identifier = strings.TrimSpace(fs.Arg(0))
	}

	if identifier == "" {
		return errors.New("torrents get requires a torrent hash, name, or index")
	}

	resolvedHash, err := resolveTorrentID(cli, opts.Timeout, identifier)

	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{
		"action": "get",
		"hash":   resolvedHash,
	}

	var out map[string]any

	if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", payload, &out, nil); err != nil {
		return err
	}

	return printJSON(out)
}

func cmdTorrentsAdd(cli *apiClient, opts globalOptions, args []string) error {
	fs := flag.NewFlagSet("torrents add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	link := fs.String("link", "", "magnet/hash/file link")
	title := fs.String("title", "", "title")
	poster := fs.String("poster", "", "poster URL")
	category := fs.String("category", "", "category")
	data := fs.String("data", "", "custom data")
	save := fs.Bool("save", false, "save torrent to db")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*link) == "" {
		return errors.New("torrents add requires --link")
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{
		"action":     "add",
		"link":       strings.TrimSpace(*link),
		"title":      strings.TrimSpace(*title),
		"poster":     strings.TrimSpace(*poster),
		"category":   strings.TrimSpace(*category),
		"data":       strings.TrimSpace(*data),
		"save_to_db": *save,
	}

	var out map[string]any

	if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", payload, &out, nil); err != nil {
		return err
	}

	return printJSON(out)
}

func cmdTorrentsHashAction(cli *apiClient, opts globalOptions, action string, args []string) error {
	// Support positional argument or --hash for backward compatibility
	fs := flag.NewFlagSet("torrents "+action, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	hash := fs.String("hash", "", "torrent hash, name, or index")

	if err := fs.Parse(args); err != nil {
		return err
	}

	identifier := strings.TrimSpace(*hash)

	if identifier == "" && len(fs.Args()) > 0 {
		identifier = strings.TrimSpace(fs.Arg(0))
	}

	if identifier == "" {
		return fmt.Errorf("torrents %s requires a torrent hash, name, or index", action)
	}

	resolvedHash, err := resolveTorrentID(cli, opts.Timeout, identifier)

	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{
		"action": action,
		"hash":   resolvedHash,
	}

	if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", payload, nil, nil); err != nil {
		return err
	}

	fmt.Printf("OK: %s %s\n", action, shortHash(resolvedHash))

	return nil
}

func cmdTorrentsWipe(cli *apiClient, opts globalOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{"action": "wipe"}

	if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", payload, nil, nil); err != nil {
		return err
	}

	fmt.Println("OK: wipe completed")

	return nil
}

func cmdSettingsDef(cli *apiClient, opts globalOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{"action": "def"}

	if err := cli.doJSON(ctx, "POST", "/api/v1/settings", payload, nil, nil); err != nil {
		return err
	}

	fmt.Println("OK: settings reset to defaults")

	return nil
}

func cmdShutdown(cli *apiClient, opts globalOptions, args []string) error {
	fs := flag.NewFlagSet("shutdown", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	mode := fs.String("mode", "local", "shutdown mode: local|public")
	reason := fs.String("reason", "tsctl", "shutdown reason")

	if err := fs.Parse(args); err != nil {
		return err
	}

	headers := map[string]string{}

	if strings.EqualFold(strings.TrimSpace(*mode), "public") {
		if strings.TrimSpace(opts.Token) == "" {
			return errors.New("shutdown --mode public requires global --token")
		}

		headers["X-TS-Shutdown-Token"] = strings.TrimSpace(opts.Token)
	}

	path := "/api/v1/shutdown"

	if strings.TrimSpace(*reason) != "" {
		path += "/" + url.PathEscape(strings.TrimSpace(*reason))
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxDuration(opts.Timeout, 5*time.Second))
	defer cancel()

	if err := cli.doJSON(ctx, "POST", path, nil, nil, headers); err != nil {
		return err
	}

	fmt.Println("OK: shutdown accepted")

	return nil
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")

	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}

	return hash[:12]
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}

	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}

	return b
}

func readSettingsPayload(jsonRaw, filePath string) (map[string]any, error) {
	if strings.TrimSpace(jsonRaw) == "" && strings.TrimSpace(filePath) == "" {
		return nil, errors.New("settings set requires --json or --file")
	}

	var data []byte

	var err error

	switch {
	case strings.TrimSpace(jsonRaw) != "":
		data = []byte(jsonRaw)
	default:
		data, err = os.ReadFile(filePath)

		if err != nil {
			return nil, fmt.Errorf("read settings file: %w", err)
		}
	}

	var out map[string]any

	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse settings json: %w", err)
	}

	return out, nil
}

func cmdSettingsGet(cli *apiClient, opts globalOptions, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{"action": "get"}

	var out map[string]any

	if err := cli.doJSON(ctx, "POST", "/api/v1/settings", payload, &out, nil); err != nil {
		return err
	}

	if key != "" {
		// Get single key
		field := findFieldByKey(key)

		if field == nil {
			// Try direct lookup
			value, ok := out[key]

			if !ok {
				return fmt.Errorf("setting %q not found", key)
			}

			fmt.Printf("%s = %v\n", key, value)

			return nil
		}

		value, ok := out[field.Key]

		if !ok {
			return fmt.Errorf("setting %q not found", field.Key)
		}

		fmt.Printf("%s = %s (%s)\n", field.Key, formatSettingsValue(value), field.Type)

		return nil
	}

	// Print all settings
	if opts.Output == "json" {
		return printJSON(out)
	}

	return printSettingsTable(out)
}

func cmdSettingsSetKeyValue(cli *apiClient, opts globalOptions, key, value string) error {
	// Find field definition
	field := findFieldByKey(key)

	if field == nil {
		return fmt.Errorf("unknown setting %q. Run 'torrserver settings get' to see available settings", key)
	}

	// Parse value
	parsed, err := parseSettingValue(field.Type, value)

	if err != nil {
		return fmt.Errorf("invalid value for %s: %w", field.Key, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{
		"action": "set",
		"sets":   map[string]any{field.Key: parsed},
	}

	if err := cli.doJSON(ctx, "POST", "/api/v1/settings", payload, nil, nil); err != nil {
		return err
	}

	fmt.Printf("OK: %s = %s\n", field.Key, formatSettingsValue(parsed))

	return nil
}

// torrentFileInfo represents a single file in a torrent for listing.
type torrentFileInfo struct {
	ID     int    `json:"id"`
	Length int64  `json:"length"`
	Path   string `json:"path"`
}

// formatFileSize returns a human-readable file size string.
func formatFileSize(bytes int64) string {
	const unit = 1024

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0

	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}

	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// cmdUrl generates and prints a streaming URL for a torrent file.

// selectFileFromTorrent finds the correct file to stream based on ID or name query.
func selectFileFromTorrent(files []torrentFileInfo, fileQuery string) (*torrentFileInfo, error) {
	if fileQuery == "" {
		// Auto-select largest file
		var selectedFile *torrentFileInfo

		var maxSize int64

		for i := range files {
			if files[i].Length > maxSize {
				maxSize = files[i].Length
				selectedFile = &files[i]
			}
		}

		if selectedFile == nil {
			return nil, errors.New("no files found in torrent")
		}

		return selectedFile, nil
	}

	// Try to parse as integer ID first
	if id, err := strconv.Atoi(fileQuery); err == nil {
		// Search by ID
		for i := range files {
			if files[i].ID == id {
				return &files[i], nil
			}
		}

		return nil, fmt.Errorf("file with ID %d not found", id)
	}

	// Search by filename substring (case-insensitive)
	query := strings.ToLower(fileQuery)

	for i := range files {
		name := strings.ToLower(files[i].Path)

		if strings.Contains(name, query) {
			return &files[i], nil
		}
	}

	return nil, fmt.Errorf("no file matching %q found", fileQuery)
}

func cmdURLWithFlags(cli *apiClient, opts globalOptions, args []string, listFiles bool, fileQuery string) error {
	if len(args) == 0 {
		return errors.New("url requires a torrent identifier (index, name, or hash)")
	}

	identifier := strings.TrimSpace(args[0])

	// Resolve torrent identifier to a hash
	hash, err := resolveTorrentID(cli, opts.Timeout, identifier)

	if err != nil {
		return err
	}

	// Get torrent details to fetch file list
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	payload := map[string]any{
		"action": "get",
		"hash":   hash,
	}

	var torr map[string]any

	if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", payload, &torr, nil); err != nil {
		return err
	}

	// Extract file_stats
	rawFiles, ok := torr["file_stats"]

	if !ok {
		return errors.New("torrent has no file stats (metadata may not be loaded)")
	}

	fileData, err := json.Marshal(rawFiles)

	if err != nil {
		return fmt.Errorf("marshal file_stats: %w", err)
	}

	var files []torrentFileInfo

	if err := json.Unmarshal(fileData, &files); err != nil {
		return fmt.Errorf("parse file stats: %w", err)
	}

	if len(files) == 0 {
		return errors.New("torrent contains no files")
	}

	// Handle --list flag
	if listFiles {
		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tSIZE\tNAME")

		for _, f := range files {
			name := f.Path

			if idx := strings.LastIndex(name, "/"); idx != -1 {
				name = name[idx+1:]
			}

			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", f.ID, formatFileSize(f.Length), name)
		}

		return w.Flush()
	}

	// Select file to stream using helper
	selectedFile, err := selectFileFromTorrent(files, fileQuery)

	if err != nil {
		return err
	}

	// Build streaming URL
	streamURL := buildStreamURL(cli.baseURL.String(), hash, selectedFile.ID)
	fmt.Println(streamURL)

	return nil
}

func buildStreamURL(base, hash string, fileID int) string {
	u, err := url.Parse(base)

	if err != nil {
		u = &url.URL{Scheme: "http", Host: hash, Path: "/"}
	}

	u.Path = "/streams/play"
	q := u.Query()
	q.Set("link", hash)
	q.Set("index", strconv.Itoa(fileID))
	u.RawQuery = q.Encode()

	return u.String()
}

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
