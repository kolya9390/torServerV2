package torrstor

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDiskWritePipelineConcurrentWrites(t *testing.T) {
	tmp := t.TempDir()
	name := filepath.Join(tmp, "piece.bin")

	p := newDiskWritePipeline(diskWriteConfig{
		syncPolicy:   "periodic",
		syncInterval: 50 * time.Millisecond,
		batchSize:    8,
	})

	chunks := [][]byte{
		[]byte("AAAA"),
		[]byte("BBBB"),
		[]byte("CCCC"),
		[]byte("DDDD"),
	}

	var wg sync.WaitGroup
	for i := range chunks {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			off := int64(i * len(chunks[i]))
			n, err := p.WriteAt(name, chunks[i], off)
			if err != nil {
				t.Errorf("write error: %v", err)
				return
			}
			if n != len(chunks[i]) {
				t.Errorf("short write: got=%d want=%d", n, len(chunks[i]))
			}
		}()
	}
	wg.Wait()
	p.Close()

	buf, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(buf) != "AAAABBBBCCCCDDDD" {
		t.Fatalf("unexpected file content: %q", string(buf))
	}
}
