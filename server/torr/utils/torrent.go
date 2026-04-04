package utils

import (
	cryptorand "crypto/rand"
	"encoding/base32"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"server/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/time/rate"
)

var defTrackers = []string{
	"http://retracker.local/announce",
	"http://bt4.t-ru.org/ann?magnet",
	"http://retracker.mgts.by:80/announce",
	"http://tracker.city9x.com:2710/announce",
	"http://tracker.electro-torrent.pl:80/announce",
	"http://tracker.internetwarriors.net:1337/announce",
	"http://tracker2.itzmx.com:6961/announce",
	"udp://opentor.org:2710",
	"udp://public.popcorn-tracker.org:6969/announce",
	"udp://tracker.opentrackr.org:1337/announce",
	"http://bt.svao-ix.ru/announce",
	"udp://explodie.org:6969/announce",
	"wss://tracker.btorrent.xyz",
	"wss://tracker.openwebtorrent.com",
}

var loadedTrackers []string
var loadedTrackersMu sync.RWMutex
var trackerRefreshCh = make(chan struct{}, 1)

func init() {
	go trackerRefreshWorker()
	triggerTrackerRefresh()
}

func GetTrackerFromFile() []string {
	name := filepath.Join(settings.Path, "trackers.txt")
	buf, err := os.ReadFile(name)
	if err == nil {
		list := strings.Split(string(buf), "\n")
		var ret []string
		for _, l := range list {
			if strings.HasPrefix(l, "udp") || strings.HasPrefix(l, "http") {
				ret = append(ret, l)
			}
		}
		return ret
	}
	return nil
}

func GetDefTrackers() []string {
	triggerTrackerRefresh()
	loadedTrackersMu.RLock()
	current := append([]string(nil), loadedTrackers...)
	loadedTrackersMu.RUnlock()
	if len(current) == 0 {
		return defTrackers
	}
	return current
}

func triggerTrackerRefresh() {
	select {
	case trackerRefreshCh <- struct{}{}:
	default:
	}
}

func trackerRefreshWorker() {
	for range trackerRefreshCh {
		loadNewTracker()
	}
}

func loadNewTracker() {
	loadedTrackersMu.RLock()
	if len(loadedTrackers) > 0 {
		loadedTrackersMu.RUnlock()
		return
	}
	loadedTrackersMu.RUnlock()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_best_ip.txt")
	if err == nil {
		defer func() {
			_ = resp.Body.Close()
		}()
		buf, err := io.ReadAll(resp.Body)
		if err == nil {
			arr := strings.Split(string(buf), "\n")
			var ret []string
			for _, s := range arr {
				s = strings.TrimSpace(s)
				if len(s) > 0 {
					ret = append(ret, s)
				}
			}
			loadedTrackersMu.Lock()
			loadedTrackers = append(ret, defTrackers...)
			loadedTrackersMu.Unlock()
		}
		return
	}
	log.Printf("failed to refresh trackers list: %v", err)
}

func PeerIDRandom(peer string) string {
	randomBytes := make([]byte, 32)
	_, _ = cryptorand.Read(randomBytes)
	return peer + base32.StdEncoding.EncodeToString(randomBytes)[:20-len(peer)]
}

func Limit(i int) *rate.Limiter {
	l := rate.NewLimiter(rate.Inf, 0)
	if i > 0 {
		b := i
		if b < 16*1024 {
			b = 16 * 1024
		}
		l = rate.NewLimiter(rate.Limit(i), b)
	}
	return l
}

func OpenTorrentFile(path string) (*torrent.TorrentSpec, error) {
	minfo, err := metainfo.LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	info, err := minfo.UnmarshalInfo()
	if err != nil {
		return nil, err
	}

	// mag := minfo.Magnet(info.Name, minfo.HashInfoBytes())
	mag := minfo.Magnet(nil, &info)
	return &torrent.TorrentSpec{
		InfoBytes:   minfo.InfoBytes,
		Trackers:    [][]string{mag.Trackers},
		DisplayName: info.Name,
		InfoHash:    minfo.HashInfoBytes(),
	}, nil
}
