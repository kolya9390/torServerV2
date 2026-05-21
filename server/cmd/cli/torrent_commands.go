package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
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
