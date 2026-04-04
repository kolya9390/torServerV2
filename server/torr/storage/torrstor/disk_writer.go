package torrstor

import (
	"io"
	"os"
	"sync"
	"time"
)

type diskWriteConfig struct {
	syncPolicy   string
	syncInterval time.Duration
	batchSize    int
}

type diskWriteReq struct {
	path string
	off  int64
	buf  []byte
	res  chan diskWriteRes
}

type diskWriteRes struct {
	n   int
	err error
}

type diskWritePipeline struct {
	cfg  diskWriteConfig
	jobs chan diskWriteReq
	wg   sync.WaitGroup
}

func newDiskWritePipeline(cfg diskWriteConfig) *diskWritePipeline {
	if cfg.syncPolicy == "" {
		cfg.syncPolicy = "periodic"
	}
	if cfg.syncInterval <= 0 {
		cfg.syncInterval = time.Second
	}
	if cfg.batchSize <= 0 {
		cfg.batchSize = 16
	}
	p := &diskWritePipeline{
		cfg:  cfg,
		jobs: make(chan diskWriteReq, 1024),
	}
	p.wg.Add(1)
	go p.loop()
	return p
}

func (p *diskWritePipeline) WriteAt(path string, b []byte, off int64) (int, error) {
	bufCopy := append([]byte(nil), b...)
	req := diskWriteReq{
		path: path,
		off:  off,
		buf:  bufCopy,
		res:  make(chan diskWriteRes, 1),
	}
	p.jobs <- req
	out := <-req.res
	return out.n, out.err
}

func (p *diskWritePipeline) Close() {
	close(p.jobs)
	p.wg.Wait()
}

func (p *diskWritePipeline) loop() {
	defer p.wg.Done()

	files := make(map[string]*os.File)
	lastSync := time.Now()

	syncTouched := func(touched map[string]struct{}) {
		if p.cfg.syncPolicy == "none" {
			return
		}
		for path := range touched {
			if ff := files[path]; ff != nil {
				_ = ff.Sync()
			}
		}
		lastSync = time.Now()
	}

	flushAll := func() {
		for _, ff := range files {
			_ = ff.Sync()
			_ = ff.Close()
		}
	}

	for req := range p.jobs {
		batch := []diskWriteReq{req}
		for len(batch) < p.cfg.batchSize {
			select {
			case next := <-p.jobs:
				batch = append(batch, next)
			default:
				goto process
			}
		}

	process:
		touched := make(map[string]struct{}, len(batch))
		for _, job := range batch {
			ff := files[job.path]
			if ff == nil {
				var err error
				ff, err = os.OpenFile(job.path, os.O_RDWR|os.O_CREATE, 0o666)
				if err != nil {
					job.res <- diskWriteRes{err: err}
					continue
				}
				files[job.path] = ff
			}

			n, err := ff.WriteAt(job.buf, job.off)
			if err == nil && n != len(job.buf) {
				err = io.ErrShortWrite
			}
			job.res <- diskWriteRes{n: n, err: err}
			if err == nil {
				touched[job.path] = struct{}{}
			}
		}

		switch p.cfg.syncPolicy {
		case "always":
			syncTouched(touched)
		case "periodic":
			if time.Since(lastSync) >= p.cfg.syncInterval {
				syncTouched(touched)
			}
		}
	}

	flushAll()
}
