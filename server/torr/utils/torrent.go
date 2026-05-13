package utils

import (
	"crypto/rand"
	"encoding/base32"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"server/log"

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
	"udp://93.158.213.92:1337/announce",
	"udp://185.121.168.96:1337/announce",
	"udp://185.243.218.213:80/announce",
	"udp://91.216.110.53:451/announce",
	"udp://44.30.4.4:6969/announce",
	"udp://23.175.184.30:23333/announce",
	"udp://209.50.255.93:3218/announce",
	"udp://135.125.236.64:6969/announce",
	"udp://91.186.213.204:6969/announce",
	"udp://43.250.54.137:6969/announce",
	"udp://212.42.38.197:6969/announce",
	"udp://81.230.84.201:6969/announce",
	"udp://173.201.36.219:6969/announce",
	"udp://209.141.59.25:6969/announce",
	"udp://5.255.124.190:6969/announce",
	"udp://193.148.251.93:6969/announce",
	"udp://37.120.182.83:1984/announce",
	"udp://211.75.210.221:6969/announce",
	"udp://34.66.57.33:80/announce",
	"udp://88.80.22.67:2710/announce",
	"http://bt.svao-ix.ru/announce",
	"udp://explodie.org:6969/announce",
	"wss://tracker.btorrent.xyz",
	"wss://tracker.openwebtorrent.com",
}

var loadedTrackers []string
var loadTrackersOnce sync.Once

const trackerListURL = "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_all_ip.txt"

func GetTrackerFromFileAtPath(basePath string) []string {
	if basePath == "" {
		basePath = "."
	}

	name := filepath.Join(basePath, "trackers.txt")

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
	loadTrackersOnce.Do(loadNewTracker)

	if len(loadedTrackers) == 0 {
		return defTrackers
	}

	return loadedTrackers
}

func NormalizeTrackers(trackers [][]string, enableIPv6 bool, maxTotal int) [][]string {
	if len(trackers) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	normalized := make([][]string, 0, len(trackers))
	total := 0

	for _, tier := range trackers {
		cleanTier := make([]string, 0, len(tier))

		for _, tracker := range tier {
			tracker = strings.TrimSpace(tracker)
			if tracker == "" {
				continue
			}

			if !enableIPv6 && trackerUsesIPv6(tracker) {
				continue
			}

			key := strings.ToLower(tracker)
			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}

			cleanTier = append(cleanTier, tracker)
			total++

			if maxTotal > 0 && total >= maxTotal {
				break
			}
		}

		if len(cleanTier) > 0 {
			normalized = append(normalized, cleanTier)
		}

		if maxTotal > 0 && total >= maxTotal {
			break
		}
	}

	return normalized
}

func trackerUsesIPv6(tracker string) bool {
	lower := strings.ToLower(tracker)
	if strings.HasPrefix(lower, "udp6://") {
		return true
	}

	parsed, err := url.Parse(tracker)
	if err != nil {
		return strings.Contains(tracker, "://[")
	}

	return strings.Contains(parsed.Hostname(), ":")
}

func loadNewTracker() {
	if len(loadedTrackers) > 0 {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(trackerListURL)
	if err == nil {
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return
		}

		buf, err := io.ReadAll(resp.Body)
		if err == nil {
			ret := parseTrackerList(string(buf))
			loadedTrackers = append(ret, defTrackers...)
		}
	}
}

func parseTrackerList(raw string) []string {
	arr := strings.Split(raw, "\n")
	ret := make([]string, 0, len(arr))

	for _, s := range arr {
		s = strings.TrimSpace(s)

		switch {
		case strings.HasPrefix(s, "udp://"),
			strings.HasPrefix(s, "http://"),
			strings.HasPrefix(s, "https://"),
			strings.HasPrefix(s, "ws://"),
			strings.HasPrefix(s, "wss://"):
			ret = append(ret, s)
		}
	}

	return ret
}

func PeerIDRandom(peer string) string {
	randomBytes := make([]byte, 32)

	_, err := rand.Read(randomBytes)
	if err != nil {
		log.TLogln("Error generating random peer ID:", err)
		// Fallback: use time-based value as a "random enough" ID
		fallback := base32.StdEncoding.EncodeToString([]byte(peer + time.Now().String()))[:20-len(peer)]
		return peer + fallback
	}

	return peer + base32.StdEncoding.EncodeToString(randomBytes)[:20-len(peer)]
}

func Limit(i int) *rate.Limiter {
	l := rate.NewLimiter(rate.Inf, 0)

	if i > 0 {
		b := max(i, 16*1024)

		l = rate.NewLimiter(rate.Limit(i), b)
	}

	return l
}

func OpenTorrentFile(path string) (*torrent.TorrentSpec, error) {
	minfo, err := metainfo.LoadFromFile(path)
	if err != nil {
		return nil, err
	}

	return torrent.TorrentSpecFromMetaInfo(minfo), nil
}
