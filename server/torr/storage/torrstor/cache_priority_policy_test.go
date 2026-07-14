package torrstor

import (
	"testing"

	"github.com/anacrolix/torrent"

	"server/settings"
)

const (
	a156NextTargetBytes     = int64(4 << 20)
	a156HighTailTargetBytes = int64(32 << 20)
)

func TestA156PriorityPieceBudgetIsStableAcrossPieceLengths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pieceLength int64
	}{
		{name: "old 512 KiB torrent", pieceLength: 512 << 10},
		{name: "old 1 MiB torrent", pieceLength: 1 << 20},
		{name: "typical 4 MiB torrent", pieceLength: 4 << 20},
		{name: "modern 8 MiB remux", pieceLength: 8 << 20},
		{name: "large 16 MiB remux", pieceLength: 16 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got, want := priorityPieceBudget(50, 1, tt.pieceLength), 80; got != want {
				t.Fatalf("single-reader priority budget = %d, want %d", got, want)
			}

			if got, want := priorityPieceBudget(50, 2, tt.pieceLength), 50; got != want {
				t.Fatalf("two-reader priority budget = %d, want %d", got, want)
			}
		})
	}
}

func TestA156ByteNormalizedAlternativeWouldChangePrioritySpread(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		pieceLength      int64
		wantNextPieces   int
		wantHighTailSize int
	}{
		{name: "old 512 KiB torrent", pieceLength: 512 << 10, wantNextPieces: 8, wantHighTailSize: 64},
		{name: "old 1 MiB torrent", pieceLength: 1 << 20, wantNextPieces: 4, wantHighTailSize: 32},
		{name: "typical 4 MiB torrent", pieceLength: 4 << 20, wantNextPieces: 1, wantHighTailSize: 8},
		{name: "modern 8 MiB remux", pieceLength: 8 << 20, wantNextPieces: 1, wantHighTailSize: 4},
		{name: "large 16 MiB remux", pieceLength: 16 << 20, wantNextPieces: 1, wantHighTailSize: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nextPieces := a156PiecesForBytes(tt.pieceLength, a156NextTargetBytes)
			if nextPieces != tt.wantNextPieces {
				t.Fatalf("byte-normalized next pieces = %d, want %d", nextPieces, tt.wantNextPieces)
			}

			highTailPieces := a156PiecesForBytes(tt.pieceLength, a156HighTailTargetBytes)
			if highTailPieces != tt.wantHighTailSize {
				t.Fatalf("byte-normalized high-tail pieces = %d, want %d", highTailPieces, tt.wantHighTailSize)
			}
		})
	}
}

func TestA156DesiredPrioritiesStayBoundedAcrossPieceLengths(t *testing.T) {
	tests := []struct {
		name        string
		pieceLength int64
	}{
		{name: "old 512 KiB torrent", pieceLength: 512 << 10},
		{name: "old 1 MiB torrent", pieceLength: 1 << 20},
		{name: "typical 4 MiB torrent", pieceLength: 4 << 20},
		{name: "modern 8 MiB remux", pieceLength: 8 << 20},
		{name: "large 16 MiB remux", pieceLength: 16 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache(512<<20, testCacheHost{
				sets: &settings.BTSets{ConnectionsLimit: 50},
			})
			cache.pieceLength = tt.pieceLength

			for id := range 200 {
				cache.pieces[id] = &Piece{ID: id}
			}

			readers := []activeReaderSnapshot{
				{
					readerPos:    10,
					readerRAHPos: 30,
					piecesRange:  Range{Start: 10, End: 200},
				},
			}
			desired := cache.desiredPrioritiesForReaders([]Range{readers[0].piecesRange}, readers)

			if got, want := len(desired), priorityPieceBudget(50, 1, tt.pieceLength); got != want {
				t.Fatalf("desired priorities = %d, want budget %d", got, want)
			}

			assertPriority(t, desired, 10, torrent.PiecePriorityNow)
			assertPriority(t, desired, 11, torrent.PiecePriorityNext)
			assertPriority(t, desired, 30, torrent.PiecePriorityReadahead)
			assertPriority(t, desired, 31, torrent.PiecePriorityHigh)
		})
	}
}

func a156PiecesForBytes(pieceLength, targetBytes int64) int {
	if pieceLength <= 0 || targetBytes <= 0 {
		return 1
	}

	pieces := int((targetBytes + pieceLength - 1) / pieceLength)
	if pieces < 1 {
		return 1
	}

	return pieces
}

func assertPriority(
	t *testing.T,
	desired map[int]torrent.PiecePriority,
	pieceID int,
	want torrent.PiecePriority,
) {
	t.Helper()

	if got := desired[pieceID]; got != want {
		t.Fatalf("desired priority for piece %d = %v, want %v", pieceID, got, want)
	}
}
