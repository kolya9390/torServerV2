package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

const torrentMetadataPollInterval = 250 * time.Millisecond

var errTorrentStoredOnly = errors.New("torrent is stored but inactive")

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

	if !listFiles {
		if fileID, parseErr := strconv.Atoi(fileQuery); parseErr == nil && fileID > 0 {
			fmt.Println(buildStreamURL(cli.baseURL.String(), hash, fileID))

			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	files, err := waitForTorrentFiles(ctx, cli, hash)
	if err != nil {
		if errors.Is(err, errTorrentStoredOnly) {
			return fmt.Errorf(
				"torrent is stored but inactive, so its file list is unavailable; use --file ID if known or activate it with `torrserver torrents add %s --save`, then retry: %w",
				hash,
				err,
			)
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf(
				"torrent metadata is not ready after %s; check peer availability and retry `torrserver url %s`: %w",
				opts.Timeout,
				hash,
				err,
			)
		}

		return err
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

			name = sanitizeTerminalText(name)
			line := fmt.Sprintf("%d\t%s\t%s\n", f.ID, formatFileSize(f.Length), name)

			if _, err := w.Write([]byte(line)); err != nil {
				return fmt.Errorf("write torrent file list: %w", err)
			}
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

func sanitizeTerminalText(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return '?'
		}

		return char
	}, value)
}

func waitForTorrentFiles(ctx context.Context, cli *apiClient, hash string) ([]torrentFileInfo, error) {
	payload := map[string]any{
		"action": "get",
		"hash":   hash,
	}

	for {
		var torr struct {
			Files       json.RawMessage `json:"file_stats"`
			Data        string          `json:"data"`
			StateString string          `json:"stat_string"`
		}

		if err := cli.doJSON(ctx, "POST", "/api/v1/torrents", payload, &torr, nil); err != nil {
			return nil, err
		}

		files, err := torrentFilesFromStatus(torr.Files, torr.Data)
		if err != nil {
			return nil, err
		}

		if len(files) > 0 {
			return files, nil
		}

		if strings.EqualFold(torr.StateString, "Torrent in db") {
			return nil, errTorrentStoredOnly
		}

		timer := time.NewTimer(torrentMetadataPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}

			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func torrentFilesFromStatus(rawFiles json.RawMessage, data string) ([]torrentFileInfo, error) {
	if len(rawFiles) > 0 && string(rawFiles) != "null" {
		var files []torrentFileInfo
		if err := json.Unmarshal(rawFiles, &files); err != nil {
			return nil, fmt.Errorf("parse torrent file list: %w", err)
		}

		if len(files) > 0 {
			return files, nil
		}
	}

	if data == "" {
		return nil, nil
	}

	var persisted struct {
		TorrServer struct {
			Files []torrentFileInfo `json:"Files"`
		} `json:"TorrServer"`
	}

	if err := json.Unmarshal([]byte(data), &persisted); err != nil {
		return nil, nil
	}

	return persisted.TorrServer.Files, nil
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
