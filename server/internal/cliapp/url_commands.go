package cliapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"server/internal/apiclient"
)

const torrentMetadataPollInterval = 250 * time.Millisecond

var errTorrentStoredOnly = errors.New("torrent is stored but inactive")

// torrentFileInfo represents a single file in a torrent for listing.
type torrentFileInfo = apiclient.TorrentFile

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

func cmdURLWithFlags(cli torrentReadAPI, opts globalOptions, identifier string, listFiles bool, fileQuery string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return errors.New("url requires a torrent identifier (index, name, or hash)")
	}

	// Resolve torrent identifier to a hash
	hash, err := resolveTorrentID(opts.commandContext(), cli, opts.Timeout, identifier)
	if err != nil {
		return err
	}

	if !listFiles {
		if fileID, parseErr := strconv.Atoi(fileQuery); parseErr == nil && fileID > 0 {
			return writeStreamURL(opts, cli.BaseURL(), hash, fileID)
		}
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	files, err := waitForTorrentFiles(ctx, cli, hash)
	if err != nil {
		if errors.Is(err, errTorrentStoredOnly) {
			return fmt.Errorf(
				"torrent is stored but inactive, so its file list is unavailable; use --file ID if known or activate it with `%s torrents add %s --save`, then retry: %w",
				opts.programName(),
				hash,
				err,
			)
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf(
				"torrent metadata is not ready after %s; check peer availability and retry `%s url %s`: %w",
				opts.Timeout,
				opts.programName(),
				hash,
				err,
			)
		}

		return err
	}

	// Handle --list flag
	if listFiles {
		if opts.Output == outputJSON {
			return writeJSONSuccess(opts.stdoutWriter(), files)
		}

		w := tabwriter.NewWriter(opts.stdoutWriter(), 2, 4, 2, ' ', 0)
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
	return writeStreamURL(opts, cli.BaseURL(), hash, selectedFile.ID)
}

func writeStreamURL(opts globalOptions, baseURL, hash string, fileID int) error {
	streamURL := redactURLCredentials(buildStreamURL(baseURL, hash, fileID))
	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), map[string]any{
			"url":          streamURL,
			"torrent_hash": hash,
			"file_id":      fileID,
		})
	}

	return writeTextLine(opts.stdoutWriter(), streamURL)
}

func waitForTorrentFiles(ctx context.Context, cli torrentReadAPI, hash string) ([]torrentFileInfo, error) {
	for {
		torr, err := cli.GetTorrent(ctx, hash)
		if err != nil {
			return nil, err
		}

		files, err := torrentFilesFromStatus(torr.FileStats, torr.Data)
		if err != nil {
			return nil, err
		}

		if len(files) > 0 {
			return files, nil
		}

		if strings.EqualFold(torr.StatString, "Torrent in db") {
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

func torrentFilesFromStatus(files []torrentFileInfo, data string) ([]torrentFileInfo, error) {
	if len(files) > 0 {
		return files, nil
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
