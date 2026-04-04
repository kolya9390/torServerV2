package torr

import "testing"

func TestGetTorrentInvalidHashNoPanic(t *testing.T) {
	if got := GetTorrent("bad"); got != nil {
		t.Fatalf("expected nil for invalid hash")
	}
}

func TestGetTorrentWithoutRuntimeNoPanic(t *testing.T) {
	oldBTS := bts
	bts = nil
	defer func() { bts = oldBTS }()

	_ = GetTorrent("0123456789abcdef0123456789abcdef01234567")
}
