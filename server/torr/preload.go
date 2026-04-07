package torr

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"server/ffprobe"

	"server/log"
	"server/settings"
	"server/torr/state"
	utils2 "server/utils"

	"github.com/anacrolix/torrent"
)

// canPreload checks if the torrent is in a state that allows preloading.
// Returns true only if the torrent is in TorrentWorking state.
func canPreload(t *Torrent) bool {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()

	return t.Stat == state.TorrentWorking
}

// isPreloadComplete checks if the preload operation should continue.
// Returns true if the torrent is still in TorrentPreload state.
func isPreloadComplete(t *Torrent) bool {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()

	return t.Stat == state.TorrentPreload
}

// monitorPreloadProgress runs a background logger that periodically logs
// preload progress and updates the expired time. It stops when the
// provided stop channel is closed or when the torrent state changes.
func (t *Torrent) monitorPreloadProgress(file *torrent.File, stopChan <-chan struct{}, timeout time.Duration) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.muTorrent.Lock()
			stat := t.Stat
			t.muTorrent.Unlock()

			if stat != state.TorrentPreload {
				return
			}

			statStr := fmt.Sprint(file.Torrent().InfoHash().HexString(), " ",
				utils2.Format(float64(t.PreloadedBytes)), "/",
				utils2.Format(float64(t.PreloadSize)), " Speed:",
				utils2.Format(t.DownloadSpeed), " Peers:",
				t.Torrent.Stats().ActivePeers, "/",
				t.Torrent.Stats().TotalPeers, " [Seeds:",
				t.Torrent.Stats().ConnectedSeeders, "]")
			log.TLogln("Preload:", statStr)
			t.AddExpiredTime(timeout)
		case <-stopChan:
			return
		}
	}
}

// runPreloadLoop performs the actual preload read operation on the provided reader.
// It reads from the reader until the specified end position or until the torrent
// state changes. Returns an error if the read operation fails (excluding EOF).
func (t *Torrent) runPreloadLoop(reader torrent.Reader, readerEnd int64) error {
	pieceLength := t.Info().PieceLength
	readahead := pieceLength * 4

	if readerEnd < readahead {
		readahead = 0
	}

	reader.SetReadahead(readahead)

	offset := int64(0)
	tmp := make([]byte, 32768)

	for offset+int64(len(tmp)) < readerEnd {
		if !isPreloadComplete(t) {
			log.TLogln("Preload cancelled")

			return nil
		}

		n, err := reader.Read(tmp)
		if err != nil {
			if err != io.EOF {
				log.TLogln("Error preload:", err)

				return err
			}

			break
		}

		offset += int64(n)

		if readahead > 0 && readerEnd-(offset+int64(len(tmp))) < readahead {
			readahead = 0

			reader.SetReadahead(0)
		}
	}

	return nil
}

// runEndRangePreloadLoop performs preload for the end range of a file.
// This is used to preload the ending portion of a file in parallel.
func (t *Torrent) runEndRangePreloadLoop(reader torrent.Reader, readerStart, readerEnd int64) error {
	reader.SetResponsive()
	reader.SetReadahead(0)

	_, err := reader.Seek(readerStart, io.SeekStart)
	if err != nil {
		log.TLogln("Err preload seek:", err)

		return err
	}

	offset := readerStart
	tmp := make([]byte, 32768)

	for offset+int64(len(tmp)) < readerEnd {
		n, err := reader.Read(tmp)
		if err != nil {
			if err != io.EOF {
				log.TLogln("Err preload read:", err)

				return err
			}

			break
		}

		offset += int64(n)

		if !isPreloadComplete(t) {
			return nil
		}
	}

	return nil
}

// probeFileMetadata uses ffprobe to extract metadata (bitrate, duration) for the file.
// This is only executed if ffprobe is available.
func (t *Torrent) probeFileMetadata(index int) {
	if !ffprobe.Exists() {
		return
	}

	link := "http://127.0.0.1:" + settings.Port + "/play/" + t.Hash().HexString() + "/" + strconv.Itoa(index)
	if settings.Ssl {
		link = "https://127.0.0.1:" + settings.SslPort + "/play/" + t.Hash().HexString() + "/" + strconv.Itoa(index)
	}

	if data, err := ffprobe.ProbeURL(link); err == nil {
		t.BitRate = data.Format.BitRate
		t.DurationSeconds = data.Format.DurationSeconds
	}
}

// preloadResult holds the outcome of a preload operation.
type preloadResult struct {
	file        *torrent.File
	readerStart torrent.Reader
	startEnd    int64
	startEndPos int64
	endStartPos int64
	endEndPos   int64
	err         error
}

// setupPreloadReaders initializes readers and calculates read ranges for preload.
// Returns a preloadResult with configured readers and range positions, or an error.
func (t *Torrent) setupPreloadReaders(file *torrent.File, size int64) preloadResult {
	result := preloadResult{file: file}

	if t.Info() == nil {
		result.err = errors.New("torrent info not available")

		return result
	}

	// Calculate start/end ranges
	result.startEnd = max(t.Info().PieceLength, 8<<20)

	result.readerStart = file.NewReader()
	if result.readerStart == nil {
		result.err = errors.New("null reader")

		return result
	}

	result.readerStart.SetResponsive()
	result.readerStart.SetReadahead(0)

	result.startEndPos = size - result.startEnd
	if result.startEndPos < 0 {
		result.startEndPos = size
	}

	if result.startEndPos > file.Length() {
		result.startEndPos = file.Length()
	}

	result.endStartPos = file.Length() - result.startEnd
	result.endEndPos = file.Length()

	return result
}

// runEndRangePreload performs the end-range portion of preload in parallel.
// Only executes if the end range starts after the start range ends.
func (t *Torrent) runEndRangePreload(result *preloadResult, wg *sync.WaitGroup, preloadErr *error) {
	if result.endStartPos <= result.startEndPos {
		return
	}

	wg.Go(func() {
		if !isPreloadComplete(t) {
			return
		}

		readerEnd := result.file.NewReader()
		if readerEnd == nil {
			log.TLogln("Err preload: null reader")

			*preloadErr = errors.New("null reader for end range")

			return
		}

		defer func() { _ = readerEnd.Close() }()

		*preloadErr = errors.Join(*preloadErr, t.runEndRangePreloadLoop(readerEnd, result.endStartPos, result.endEndPos))
	})
}

// runPreloadSequence orchestrates the complete preload operation including readers,
// parallel end-range preload, metadata probing, and progress monitoring.
func (t *Torrent) runPreloadSequence(file *torrent.File, size int64, index int) error {
	setup := t.setupPreloadReaders(file, size)
	if setup.err != nil {
		log.TLogln("End preload:", setup.err)

		return setup.err
	}

	defer func() { _ = setup.readerStart.Close() }()

	timeout := min(time.Second*time.Duration(settings.BTsets.TorrentDisconnectTimeout), time.Minute)

	// Create a stop channel for the logging goroutine
	logStopChan := make(chan struct{})
	defer close(logStopChan)

	// Start progress monitoring in background
	go t.monitorPreloadProgress(file, logStopChan, timeout)

	// Probe file metadata
	t.probeFileMetadata(index)

	// Check if torrent was closed
	if !isPreloadComplete(t) {
		log.TLogln("End preload: torrent closed")

		return nil
	}

	var wg sync.WaitGroup

	var preloadErr error

	// Start end range preload if needed
	t.runEndRangePreload(&setup, &wg, &preloadErr)

	// Main preload section
	preloadErr = errors.Join(preloadErr, t.runPreloadLoop(setup.readerStart, setup.startEndPos))

	// Wait for end range preload to complete
	wg.Wait()

	// Check if end range preload failed
	if preloadErr != nil {
		log.TLogln("End range preload failed:", preloadErr)
	}

	// Final log
	if isPreloadComplete(t) {
		log.TLogln("End preload:", file.Torrent().InfoHash().HexString(),
			"Peers:", t.Torrent.Stats().ActivePeers, "/",
			t.Torrent.Stats().TotalPeers, "[ Seeds:",
			t.Torrent.Stats().ConnectedSeeders, "]")
	}

	return nil
}

// Preload downloads the beginning and optionally the end of a torrent file
// to enable faster playback start. It handles state transitions, progress
// logging, and metadata extraction.
func (t *Torrent) Preload(index int, size int64) {
	if size <= 0 {
		return
	}

	t.PreloadSize = size

	if t.Stat == state.TorrentGettingInfo {
		if !t.WaitInfo() {
			return
		}
		// wait change status
		time.Sleep(100 * time.Millisecond)
	}

	if !canPreload(t) {
		return
	}

	t.muTorrent.Lock()
	t.Stat = state.TorrentPreload
	t.muTorrent.Unlock()

	defer func() {
		t.muTorrent.Lock()
		if t.Stat == state.TorrentPreload {
			t.Stat = state.TorrentWorking
		}
		t.muTorrent.Unlock()
		// Clear on preload completion
		t.BitRate = ""
		t.DurationSeconds = 0
	}()

	file := t.findFileIndex(index)
	if file == nil {
		file = t.Files()[0]
	}

	if size > file.Length() {
		size = file.Length()
	}

	// Run the main preload sequence
	if err := t.runPreloadSequence(file, size, index); err != nil {
		log.TLogln("Preload error:", err)
	}
}

func (t *Torrent) findFileIndex(index int) *torrent.File {
	st := t.Status()

	var stFile *state.TorrentFileStat

	for _, f := range st.FileStats {
		if index == f.Id {
			stFile = f

			break
		}
	}

	if stFile == nil {
		return nil
	}

	for _, file := range t.Files() {
		if file.Path() == stFile.Path {
			return file
		}
	}

	return nil
}
