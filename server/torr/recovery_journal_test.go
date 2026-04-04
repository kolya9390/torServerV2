package torr

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParsePendingJournalEntries(t *testing.T) {
	now := time.Now().UTC()
	lines := []journalEntry{
		{ID: "1", Timestamp: now.Add(1 * time.Second), Operation: journalOpSaveTorrentDB, Phase: journalPhaseBegin},
		{ID: "2", Timestamp: now.Add(2 * time.Second), Operation: journalOpDropTorrent, Phase: journalPhaseBegin},
		{ID: "1", Timestamp: now.Add(3 * time.Second), Operation: journalOpSaveTorrentDB, Phase: journalPhaseCommit},
	}
	var data []byte
	for _, line := range lines {
		b, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		data = append(data, b...)
		data = append(data, '\n')
	}

	pending := parsePendingJournalEntries(data)
	if len(pending) != 1 {
		t.Fatalf("expected one pending entry, got %d", len(pending))
	}
	if pending[0].ID != "2" {
		t.Fatalf("expected pending id=2, got %s", pending[0].ID)
	}
}
