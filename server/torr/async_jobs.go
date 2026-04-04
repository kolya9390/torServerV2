package torr

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anacrolix/torrent"

	"server/log"
	"server/settings"
)

type metadataJob struct {
	jobType metadataJobType
	tor     *Torrent

	// finalize metadata job payload
	displayName string
	saveToDB    bool
}

type preloadJob struct {
	tor   *Torrent
	index int
	key   string
}

type asyncWorkerConfig struct {
	metadataWorkers int
	metadataQueue   int
	preloadWorkers  int
	preloadQueue    int
}

type metadataJobType int

const (
	metadataFinalizeJob metadataJobType = iota + 1
	metadataActivateJob
)

type AsyncQueuesSnapshot struct {
	MetadataQueueLen int
	MetadataQueueCap int
	PreloadQueueLen  int
	PreloadQueueCap  int
	PreloadPending   int
	MetadataDropped  uint64
	PreloadDropped   uint64
}

var (
	asyncWorkersOnce sync.Once

	metadataJobs chan metadataJob
	preloadJobs  chan preloadJob

	preloadPendingMu sync.Mutex
	preloadPending   = make(map[string]struct{})

	metadataDropped atomic.Uint64
	preloadDropped  atomic.Uint64
)

func currentAsyncWorkerConfig() asyncWorkerConfig {
	cfg := asyncWorkerConfig{
		metadataWorkers: maxInt(2, runtime.GOMAXPROCS(0)/2),
		metadataQueue:   256,
		preloadWorkers:  1,
		preloadQueue:    32,
	}
	if settings.BTsets != nil {
		if settings.BTsets.MetadataWorkers > 0 {
			cfg.metadataWorkers = settings.BTsets.MetadataWorkers
		}
		if settings.BTsets.MetadataQueueSize > 0 {
			cfg.metadataQueue = settings.BTsets.MetadataQueueSize
		}
		if settings.BTsets.PreloadWorkers > 0 {
			cfg.preloadWorkers = settings.BTsets.PreloadWorkers
		}
		if settings.BTsets.PreloadQueueSize > 0 {
			cfg.preloadQueue = settings.BTsets.PreloadQueueSize
		}
	}
	cfg.metadataWorkers = maxInt(1, cfg.metadataWorkers)
	cfg.metadataQueue = maxInt(1, cfg.metadataQueue)
	cfg.preloadWorkers = maxInt(1, cfg.preloadWorkers)
	cfg.preloadQueue = maxInt(1, cfg.preloadQueue)
	return cfg
}

func ensureAsyncWorkers() {
	asyncWorkersOnce.Do(func() {
		cfg := currentAsyncWorkerConfig()
		metadataJobs = make(chan metadataJob, cfg.metadataQueue)
		preloadJobs = make(chan preloadJob, cfg.preloadQueue)

		for i := 0; i < cfg.metadataWorkers; i++ {
			go metadataWorkerLoop()
		}
		for i := 0; i < cfg.preloadWorkers; i++ {
			go preloadWorkerLoop()
		}
	})
}

func EnqueueMetadataFinalize(tor *Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool {
	if tor == nil {
		return false
	}
	ensureAsyncWorkers()

	displayName := ""
	if spec != nil {
		displayName = spec.DisplayName
	}

	job := metadataJob{
		jobType:     metadataFinalizeJob,
		tor:         tor,
		displayName: displayName,
		saveToDB:    saveToDB,
	}
	select {
	case metadataJobs <- job:
		return true
	default:
		metadataDropped.Add(1)
		log.TLogln("metadata queue overflow: dropping async metadata finalize task")
		return false
	}
}

func EnqueueTorrentActivateFromDB(tor *Torrent) bool {
	if tor == nil {
		return false
	}
	ensureAsyncWorkers()
	job := metadataJob{
		jobType: metadataActivateJob,
		tor:     tor,
	}
	select {
	case metadataJobs <- job:
		return true
	default:
		metadataDropped.Add(1)
		log.TLogln("metadata queue overflow: dropping torrent activation task")
		return false
	}
}

func EnqueuePreload(tor *Torrent, index int) bool {
	if tor == nil {
		return false
	}
	ensureAsyncWorkers()

	key := preloadKey(tor, index)
	preloadPendingMu.Lock()
	if _, exists := preloadPending[key]; exists {
		preloadPendingMu.Unlock()
		return true
	}
	preloadPending[key] = struct{}{}
	preloadPendingMu.Unlock()

	job := preloadJob{tor: tor, index: index, key: key}
	select {
	case preloadJobs <- job:
		return true
	default:
		preloadPendingMu.Lock()
		delete(preloadPending, key)
		preloadPendingMu.Unlock()
		preloadDropped.Add(1)
		log.TLogln("preload queue overflow: dropping preload task")
		return false
	}
}

func metadataWorkerLoop() {
	for job := range metadataJobs {
		switch job.jobType {
		case metadataFinalizeJob:
			processMetadataFinalize(job)
		case metadataActivateJob:
			processMetadataActivation(job.tor)
		}
	}
}

func processMetadataFinalize(job metadataJob) {
	if !job.tor.GotInfo() {
		log.TLogln("async metadata finalize: timeout waiting torrent info")
		return
	}
	job.tor.muTorrent.Lock()
	if job.tor.Title == "" {
		title := sanitizeDisplayTitle(job.displayName)
		if title == "" {
			title = job.tor.Name()
		}
		job.tor.Title = title
	}
	job.tor.muTorrent.Unlock()

	if job.saveToDB {
		SaveTorrentToDB(job.tor)
	}
}

func processMetadataActivation(tor *Torrent) {
	if tor == nil {
		return
	}
	log.TLogln("New torrent", tor.Hash())
	tr, _ := NewTorrent(tor.TorrentSpec, bts)
	if tr == nil {
		return
	}
	tr.Title = tor.Title
	tr.Poster = tor.Poster
	tr.Data = tor.Data
	tr.Size = tor.Size
	tr.Timestamp = tor.Timestamp
	tr.Category = tor.Category
	
	// Wait for torrent info with timeout
	if tr.GotInfo() {
		log.TLogln("Got torrent info:", tr.Title)
	} else {
		log.TLogln("Timeout waiting for torrent info:", tor.Hash())
	}
}

func preloadWorkerLoop() {
	for job := range preloadJobs {
		job.tor.Preload(job.index, estimatePreloadSize())
		preloadPendingMu.Lock()
		delete(preloadPending, job.key)
		preloadPendingMu.Unlock()
	}
}

func preloadKey(tor *Torrent, index int) string {
	return tor.Hash().HexString() + ":" + strconv.Itoa(index)
}

func sanitizeDisplayTitle(title string) string {
	if title == "" {
		return ""
	}
	title = strings.ReplaceAll(title, "rutor.info", "")
	title = strings.ReplaceAll(title, "_", " ")
	return strings.TrimSpace(title)
}

func estimatePreloadSize() int64 {
	cache := float32(settings.BTsets.CacheSize)
	preload := float32(settings.BTsets.PreloadCache)
	size := int64((cache / 100.0) * preload)
	if size <= 0 {
		return 0
	}
	if size > settings.BTsets.CacheSize {
		return settings.BTsets.CacheSize
	}
	return size
}

func GetAsyncQueuesSnapshot() AsyncQueuesSnapshot {
	ensureAsyncWorkers()

	preloadPendingMu.Lock()
	pending := len(preloadPending)
	preloadPendingMu.Unlock()

	return AsyncQueuesSnapshot{
		MetadataQueueLen: len(metadataJobs),
		MetadataQueueCap: cap(metadataJobs),
		PreloadQueueLen:  len(preloadJobs),
		PreloadQueueCap:  cap(preloadJobs),
		PreloadPending:   pending,
		MetadataDropped:  metadataDropped.Load(),
		PreloadDropped:   preloadDropped.Load(),
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
