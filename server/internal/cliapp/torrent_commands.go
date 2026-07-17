package cliapp

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"server/internal/apiclient"
)

type torrentStatus = apiclient.Torrent

const (
	maxTorrentUploadBodyBytes   int64 = 4 << 20
	multipartUploadReserveBytes int64 = 64 << 10
	maxTorrentUploadFileBytes         = maxTorrentUploadBodyBytes - multipartUploadReserveBytes
)

type torrentAddOptions struct {
	Source     string
	Link       string
	File       string
	Title      string
	Poster     string
	Category   string
	Data       string
	Save       bool
	fileSystem FileSystem
}

func cmdTorrentsList(cli torrentReadAPI, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	out, err := cli.ListTorrents(ctx)
	if err != nil {
		return err
	}

	if opts.Output == "json" {
		return writeJSONSuccess(opts.stdoutWriter(), out)
	}

	w := tabwriter.NewWriter(opts.stdoutWriter(), 2, 4, 2, ' ', 0)
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
func resolveTorrentID(parent context.Context, cli torrentReadAPI, timeout time.Duration, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)

	if identifier == "" {
		return "", errors.New("torrent identifier is required")
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	torrents, err := cli.ListTorrents(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch torrent list: %w", err)
	}

	// Try as numeric index (1-based)
	if idx, err := strconv.Atoi(identifier); err == nil {
		if idx < 1 || idx > len(torrents) {
			return "", fmt.Errorf("index %d out of range (1-%d)", idx, len(torrents))
		}

		return canonicalTorrentHash(torrents[idx-1].Hash)
	}

	// Try as full hash (40 hex chars)
	if len(identifier) == 40 {
		for _, t := range torrents {
			if strings.EqualFold(t.Hash, identifier) {
				return canonicalTorrentHash(t.Hash)
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

	return canonicalTorrentHash(matches[0].Hash)
}

func canonicalTorrentHash(value string) (string, error) {
	if len(value) != 40 {
		return "", errors.New("server returned a torrent hash with invalid length")
	}

	decoded, err := hex.DecodeString(value)
	if err != nil {
		return "", errors.New("server returned a non-hex torrent hash")
	}

	return hex.EncodeToString(decoded), nil
}

func cmdTorrentsGet(cli torrentReadAPI, opts globalOptions, identifier string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return errors.New("torrents get requires a torrent hash, name, or index")
	}

	resolvedHash, err := resolveTorrentID(opts.commandContext(), cli, opts.Timeout, identifier)

	if err != nil {
		return err
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	out, err := cli.GetTorrent(ctx, resolvedHash)
	if err != nil {
		return err
	}

	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), out)
	}

	return writeJSON(opts.stdoutWriter(), out)
}

func cmdTorrentsAdd(cli torrentAddAPI, opts globalOptions, addOpts torrentAddOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	if addOpts.File != "" {
		return uploadTorrentFile(ctx, cli, opts, addOpts)
	}

	out, err := cli.AddTorrent(ctx, apiclient.AddTorrentRequest{
		Link:     addOpts.Source,
		Title:    addOpts.Title,
		Poster:   addOpts.Poster,
		Category: addOpts.Category,
		Data:     addOpts.Data,
		Save:     addOpts.Save,
	})
	if err != nil {
		return err
	}

	return printTorrentAdded(out, opts)
}

func resolveTorrentAddOptions(args []string, opts torrentAddOptions) (torrentAddOptions, error) {
	if opts.fileSystem == nil {
		opts.fileSystem = osFileSystem{}
	}

	positional := ""
	if len(args) > 0 {
		positional = strings.TrimSpace(args[0])
	}

	link := strings.TrimSpace(opts.Link)
	filePath := strings.TrimSpace(opts.File)
	sourceCount := 0

	sources := []string{positional, link, filePath}
	for _, source := range sources {
		if source != "" {
			sourceCount++
		}
	}

	if sourceCount == 0 {
		return torrentAddOptions{}, errors.New("torrents add requires a magnet, hash, link, or local .torrent file")
	}

	if sourceCount > 1 {
		return torrentAddOptions{}, errors.New("provide exactly one torrent source: positional argument, --link, or --file")
	}

	opts.Source = firstNonEmpty(positional, firstNonEmpty(link, filePath))
	opts.Link = ""
	opts.File = ""
	opts.Title = strings.TrimSpace(opts.Title)
	opts.Poster = strings.TrimSpace(opts.Poster)
	opts.Category = strings.TrimSpace(opts.Category)
	opts.Data = strings.TrimSpace(opts.Data)

	if filePath != "" {
		opts.File = filePath

		return validateTorrentUploadFile(opts)
	}

	localPath, local, err := resolveLocalTorrentPath(opts.fileSystem, opts.Source)
	if err != nil {
		return torrentAddOptions{}, err
	}

	if local {
		opts.File = localPath
	}

	return validateTorrentUploadFile(opts)
}

func resolveLocalTorrentPath(fileSystem FileSystem, source string) (string, bool, error) {
	parsed, err := url.Parse(source)
	if err == nil && parsed.Scheme == "file" {
		if parsed.Host != "" && parsed.Host != "localhost" {
			return "", false, fmt.Errorf("file URI host %q is not local", parsed.Host)
		}

		path, unescapeErr := url.PathUnescape(parsed.Path)
		if unescapeErr != nil {
			return "", false, fmt.Errorf("decode file URI: %w", unescapeErr)
		}

		return path, true, nil
	}

	if err == nil && parsed.Scheme != "" {
		return "", false, nil
	}

	_, statErr := fileSystem.Stat(source)
	if statErr == nil {
		return source, true, nil
	}

	if !errors.Is(statErr, fs.ErrNotExist) {
		return "", false, fmt.Errorf("inspect torrent source: %w", statErr)
	}

	if strings.EqualFold(filepath.Ext(source), ".torrent") || strings.ContainsRune(source, filepath.Separator) {
		return "", false, fmt.Errorf("local torrent file %q does not exist", source)
	}

	return "", false, nil
}

func validateTorrentUploadFile(opts torrentAddOptions) (torrentAddOptions, error) {
	if opts.File == "" {
		return opts, nil
	}

	info, err := opts.fileSystem.Stat(opts.File)
	if err != nil {
		return torrentAddOptions{}, fmt.Errorf("inspect torrent file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return torrentAddOptions{}, fmt.Errorf("torrent file %q is not a regular file", opts.File)
	}

	if info.Size() <= 0 {
		return torrentAddOptions{}, fmt.Errorf("torrent file %q is empty", opts.File)
	}

	if info.Size() >= maxTorrentUploadFileBytes {
		return torrentAddOptions{}, fmt.Errorf(
			"torrent file %q is too large: %d bytes (maximum file size is %d bytes)",
			opts.File,
			info.Size(),
			maxTorrentUploadFileBytes-1,
		)
	}

	return opts, nil
}

func uploadTorrentFile(ctx context.Context, cli torrentAddAPI, opts globalOptions, addOpts torrentAddOptions) error {
	out, err := cli.UploadTorrent(ctx, apiclient.UploadTorrentRequest{
		FilePath: addOpts.File,
		Title:    addOpts.Title,
		Poster:   addOpts.Poster,
		Category: addOpts.Category,
		Data:     addOpts.Data,
		Save:     addOpts.Save,
	})
	if err != nil {
		return err
	}

	return printTorrentAdded(out, opts)
}

func printTorrentAdded(out torrentStatus, opts globalOptions) error {
	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), out)
	}

	return writeTextLines(opts.stdoutWriter(), strings.Split(torrentAddedMessage(out, opts), "\n")...)
}

func torrentAddedMessage(out torrentStatus, opts globalOptions) string {
	title := out.Title
	hash := out.Hash
	lines := make([]string, 0, 3)

	if title != "" {
		lines = append(lines, "Added: "+title)
	} else {
		lines = append(lines, "Torrent added")
	}

	if hash == "" {
		return strings.Join(append(
			lines,
			fmt.Sprintf(
				"Next: run `%s torrents list`, then `%s url <INDEX|NAME|HASH>`",
				opts.programName(),
				opts.programName(),
			),
		), "\n")
	}

	lines = append(lines, "Hash: "+hash, "Next: "+streamURLCommand(opts, hash))

	return strings.Join(lines, "\n")
}

func streamURLCommand(opts globalOptions, hash string) string {
	if opts.Context != "" {
		return fmt.Sprintf("%s --context %s url %s", opts.programName(), opts.Context, hash)
	}

	if opts.Server != "" && strings.TrimRight(opts.Server, "/") != defaultServerURL {
		return fmt.Sprintf("%s --server %s url %s", opts.programName(), opts.Server, hash)
	}

	return opts.programName() + " url " + hash
}

func cmdTorrentsHashAction(cli torrentMutationAPI, opts globalOptions, action, identifier string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return fmt.Errorf("torrents %s requires a torrent hash, name, or index", action)
	}

	resolvedHash, err := resolveTorrentID(opts.commandContext(), cli, opts.Timeout, identifier)

	if err != nil {
		return err
	}

	//nolint:gosec // resolvedHash is canonical lowercase hex returned by canonicalTorrentHash.
	_, _ = fmt.Fprintln(opts.stderrWriter(), "Resolved torrent target:", resolvedHash)

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	var actionErr error
	switch action {
	case "rem":
		actionErr = cli.RemoveTorrent(ctx, resolvedHash)
	case "drop":
		actionErr = cli.DropTorrent(ctx, resolvedHash)
	default:
		return fmt.Errorf("unsupported torrent action %q", action)
	}
	if actionErr != nil {
		return actionErr
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": action, "hash": resolvedHash},
		fmt.Sprintf("OK: %s %s", action, shortHash(resolvedHash)),
	)
}

func cmdTorrentsWipe(cli torrentWipeAPI, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	if err := cli.WipeTorrents(ctx); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "torrents_wiped"},
		"OK: wipe completed",
	)
}
