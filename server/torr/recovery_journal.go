package torr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"server/log"
	"server/settings"
)

const (
	journalOpSaveTorrentDB      = "save_torrent_db"
	journalOpRemoveTorrentDB    = "remove_torrent_db"
	journalOpDropTorrent        = "drop_torrent"
	journalOpSetSettings        = "set_settings"
	journalOpSetDefaultSettings = "set_default_settings"
	journalOpSetStoragePrefs    = "set_storage_prefs"

	journalPhaseBegin  = "begin"
	journalPhaseCommit = "commit"
)

type journalEntry struct {
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"ts"`
	Operation string          `json:"op"`
	Phase     string          `json:"phase"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type hashPayload struct {
	Hash string `json:"hash"`
}

var (
	journalMu         sync.Mutex
	journalIDCounter  atomic.Uint64
	journalReplayMode atomic.Bool
	journalReplayOnce sync.Once
)

func ensureJournalRecovered() {
	journalReplayOnce.Do(func() {
		replayRecoveryJournal()
	})
}

func beginJournalOperation(operation string, payload any) string {
	if journalReplayMode.Load() {
		return ""
	}
	entry := journalEntry{
		ID:        nextJournalID(),
		Timestamp: time.Now().UTC(),
		Operation: operation,
		Phase:     journalPhaseBegin,
	}
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err == nil {
			entry.Payload = buf
		}
	}
	if err := appendJournalEntry(entry); err != nil {
		log.TLogln("journal begin append error:", err)
	}
	return entry.ID
}

func commitJournalOperation(id, operation string, payload any) {
	if id == "" || journalReplayMode.Load() {
		return
	}
	entry := journalEntry{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Operation: operation,
		Phase:     journalPhaseCommit,
	}
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err == nil {
			entry.Payload = buf
		}
	}
	if err := appendJournalEntry(entry); err != nil {
		log.TLogln("journal commit append error:", err)
	}
}

func appendJournalEntry(entry journalEntry) error {
	journalMu.Lock()
	defer journalMu.Unlock()

	path := recoveryJournalPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ff, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = ff.Close()
	}()

	buf, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := ff.Write(append(buf, '\n')); err != nil {
		return err
	}
	return ff.Sync()
}

func replayRecoveryJournal() {
	path := recoveryJournalPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || len(data) == 0 {
		return
	}
	if err != nil {
		log.TLogln("journal replay read error:", err)
		return
	}

	pending := parsePendingJournalEntries(data)
	if len(pending) == 0 {
		_ = os.Truncate(path, 0)
		return
	}

	log.TLogln("journal replay: pending operations", len(pending))
	journalReplayMode.Store(true)
	defer journalReplayMode.Store(false)

	failed := make([]journalEntry, 0)
	for _, entry := range pending {
		if err := applyJournalOperation(entry); err != nil {
			log.TLogln("journal replay op failed:", entry.Operation, err)
			failed = append(failed, entry)
			continue
		}
	}
	if len(failed) == 0 {
		_ = os.Truncate(path, 0)
		return
	}

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	for _, entry := range failed {
		if err := enc.Encode(entry); err != nil {
			log.TLogln("journal replay encode error:", err)
			return
		}
	}
	_ = os.WriteFile(path, out.Bytes(), 0o644)
}

func parsePendingJournalEntries(data []byte) []journalEntry {
	begins := make(map[string]journalEntry)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var entry journalEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || entry.ID == "" {
			continue
		}
		switch entry.Phase {
		case journalPhaseBegin:
			begins[entry.ID] = entry
		case journalPhaseCommit:
			delete(begins, entry.ID)
		}
	}
	pending := make([]journalEntry, 0, len(begins))
	for _, entry := range begins {
		pending = append(pending, entry)
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Timestamp.Before(pending[j].Timestamp)
	})
	return pending
}

func applyJournalOperation(entry journalEntry) error {
	switch entry.Operation {
	case journalOpSaveTorrentDB:
		var snap settings.TorrentDB
		if err := json.Unmarshal(entry.Payload, &snap); err != nil {
			return err
		}
		settings.AddTorrent(&snap)
		return nil
	case journalOpRemoveTorrentDB:
		var payload hashPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return err
		}
		hash, ok := parseHashHexSafe(payload.Hash)
		if !ok {
			return fmt.Errorf("invalid hash %q", payload.Hash)
		}
		settings.RemTorrent(hash)
		return nil
	case journalOpDropTorrent:
		var payload hashPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return err
		}
		DropTorrent(payload.Hash)
		return nil
	case journalOpSetSettings:
		var cfg settings.BTSets
		if err := json.Unmarshal(entry.Payload, &cfg); err != nil {
			return err
		}
		SetSettings(&cfg)
		return nil
	case journalOpSetDefaultSettings:
		SetDefSettings()
		return nil
	case journalOpSetStoragePrefs:
		var prefs map[string]interface{}
		if err := json.Unmarshal(entry.Payload, &prefs); err != nil {
			return err
		}
		return settings.SetStoragePreferences(prefs)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func snapshotTorrentDB(torr *Torrent) *settings.TorrentDB {
	if torr == nil || torr.TorrentSpec == nil {
		return nil
	}
	specCopy := *torr.TorrentSpec
	return &settings.TorrentDB{
		TorrentSpec: &specCopy,
		Title:       torr.Title,
		Category:    torr.Category,
		Poster:      torr.Poster,
		Data:        torr.Data,
		Timestamp:   torr.Timestamp,
		Size:        torr.Size,
	}
}

func recoveryJournalPath() string {
	base := settings.Path
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "core_recovery_journal.jsonl")
}

func nextJournalID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), journalIDCounter.Add(1))
}

func parseHashHexSafe(hashHex string) (hash metainfo.Hash, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	hash = metainfo.NewHashFromHex(hashHex)
	return hash, true
}

func cloneBTSets(src *settings.BTSets) *settings.BTSets {
	if src == nil {
		return nil
	}
	cp := *src
	cp.TorznabUrls = append([]settings.TorznabConfig(nil), src.TorznabUrls...)
	cp.ProxyHosts = append([]string(nil), src.ProxyHosts...)
	return &cp
}
