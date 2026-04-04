package torr

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

func TestBTServerMapAccessConcurrency(t *testing.T) {
	bt := NewBTS()

	var wg sync.WaitGroup

	// Writers.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				var h metainfo.Hash
				h[0] = byte((offset + j) % 255)
				bt.mu.Lock()
				bt.torrents[h] = &Torrent{}
				bt.mu.Unlock()
				bt.RemoveTorrent(h)
			}
		}(i)
	}

	// Readers.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				var h metainfo.Hash
				h[0] = byte(j % 255)
				_ = bt.GetTorrent(h)
				_ = bt.ListTorrents()
			}
		}()
	}

	wg.Wait()
}

func TestTorrentCloseIsIdempotent(t *testing.T) {
	bt := NewBTS()
	torr := &Torrent{bt: bt}
	var h metainfo.Hash
	h[0] = 7

	bt.mu.Lock()
	bt.torrents[h] = torr
	bt.mu.Unlock()

	var okCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if torr.Close() {
				okCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if okCount.Load() != 1 {
		t.Fatalf("expected exactly one successful close, got %d", okCount.Load())
	}
}
