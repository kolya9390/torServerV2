package torznab

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"server/log"
	"server/settings"
	"server/utils/circuitbreaker"
	"server/utils/retry"
)

var (
	breaker   *circuitbreaker.CircuitBreaker
	breakerMu sync.Mutex
)

type TorrentDetails struct {
	Title      string    `json:"title,omitempty"`
	Name       string    `json:"name,omitempty"`
	Link       string    `json:"link,omitempty"`
	Magnet     string    `json:"magnet,omitempty"`
	Hash       string    `json:"hash,omitempty"`
	Size       string    `json:"size,omitempty"`
	Seed       int       `json:"seed,omitempty"`
	Peer       int       `json:"peer,omitempty"`
	CreateDate time.Time `json:"createDate"`
	Categories []string  `json:"categories,omitempty"`
	Year       int       `json:"year,omitempty"`
}

type TorznabAttribute struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type TorznabEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type TorznabItem struct {
	Title       string             `xml:"title"`
	Link        string             `xml:"link"`
	Description string             `xml:"description"`
	PubDate     string             `xml:"pubDate"`
	Size        int64              `xml:"size"`
	Enclosure   []TorznabEnclosure `xml:"enclosure"`
	Attributes  []TorznabAttribute `xml:"attr"`
}

type TorznabChannel struct {
	Items []TorznabItem `xml:"item"`
}

type TorznabResponse struct {
	Channel TorznabChannel `xml:"channel"`
}

func getBreaker() *circuitbreaker.CircuitBreaker {
	breakerMu.Lock()
	defer breakerMu.Unlock()

	if breaker == nil {
		breaker = circuitbreaker.New("torznab", circuitbreaker.Config{
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          30 * time.Second,
			OnStateChange: func(oldState, newState circuitbreaker.State) {
				log.Warn("torznab_circuit_breaker",
					"old_state", oldState.String(),
					"new_state", newState.String())
			},
			OnRequestBlocked: func() {
				log.Warn("torznab_request_blocked", "reason", "circuit_open")
			},
		})
	}

	return breaker
}

func Search(query string, index int) []*TorrentDetails {
	if !settings.BTsets.EnableTorznabSearch || len(settings.BTsets.TorznabUrls) == 0 {
		return nil
	}

	var allResults []*TorrentDetails

	if index >= 0 && index < len(settings.BTsets.TorznabUrls) {
		config := settings.BTsets.TorznabUrls[index]
		if config.Host != "" && config.Key != "" {
			return searchOne(config.Host, config.Key, query)
		}

		return nil
	}

	for _, config := range settings.BTsets.TorznabUrls {
		if config.Host == "" || config.Key == "" {
			continue
		}

		results := searchOne(config.Host, config.Key, query)
		if results != nil {
			allResults = append(allResults, results...)
		}
	}

	return allResults
}

func searchOne(host, key, query string) []*TorrentDetails {
	cb := getBreaker()

	var results []*TorrentDetails

	err := cb.Execute(func() error {
		var err error
		results, err = doSearchOne(host, key, query)

		return err
	})

	if err != nil {
		if strings.Contains(err.Error(), "circuit_open") {
			log.Warn("torznab_search_skipped", "reason", "circuit_open", "host", host)
		}

		return nil
	}

	return results
}

// buildSearchRequest constructs an HTTP GET request for the Torznab search API.
// It ensures the host URL has a trailing slash and appends the required query parameters.
func buildSearchRequest(host, key, query string) (*http.Request, error) {
	if !strings.HasSuffix(host, "/") {
		host += "/"
	}

	u, err := url.Parse(host + "api")
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	q := u.Query()
	q.Set("apikey", key)
	q.Set("t", "search")
	q.Set("q", query)
	q.Set("cat", "5000,2000")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}

	return req, nil
}

// parseSearchResponse decodes the XML response from the Torznab API and extracts torrent details.
// It handles enclosure URLs, custom attributes (magneturl, seeders, peers), and magnet link extraction.
func parseSearchResponse(resp *http.Response) ([]*TorrentDetails, error) {
	var torznabResp TorznabResponse
	if err := xml.NewDecoder(resp.Body).Decode(&torznabResp); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}

	var results []*TorrentDetails

	for _, item := range torznabResp.Channel.Items {
		detail := parseItem(item)
		results = append(results, detail)
	}

	return results, nil
}

// parseItem extracts TorrentDetails from a single TorznabItem.
func parseItem(item TorznabItem) *TorrentDetails {
	detail := &TorrentDetails{
		Title:      item.Title,
		Name:       item.Title,
		Link:       item.Link,
		CreateDate: parseDate(item.PubDate),
	}

	if len(item.Enclosure) > 0 {
		detail.Link = item.Enclosure[0].URL
		detail.Size = formatSize(item.Enclosure[0].Length)
	} else {
		detail.Size = formatSize(item.Size)
	}

	for _, attr := range item.Attributes {
		switch attr.Name {
		case "magneturl":
			detail.Magnet = attr.Value
			detail.Hash = extractHash(detail.Magnet)
		case "seeders":
			if val, err := strconv.Atoi(attr.Value); err == nil {
				detail.Seed = val
			}
		case "peers":
			if val, err := strconv.Atoi(attr.Value); err == nil {
				detail.Peer = val
			}
		}
	}

	if detail.Magnet == "" && strings.HasPrefix(detail.Link, "magnet:") {
		detail.Magnet = detail.Link
		detail.Hash = extractHash(detail.Magnet)
	}

	return detail
}

func doSearchOne(host, key, query string) ([]*TorrentDetails, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := retry.DefaultConfig
	cfg.MaxAttempts = 3

	result := retry.Do[[]*TorrentDetails](ctx, cfg, func() ([]*TorrentDetails, error) {
		req, err := buildSearchRequest(host, key, query)
		if err != nil {
			return nil, err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request error: %w", err)
		}

		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status: %d", resp.StatusCode)
		}

		return parseSearchResponse(resp)
	})

	if result.Err != nil {
		log.Warn("torznab_search_failed", "error", result.Err, "host", host)

		return nil, result.Err
	}

	return result.Value, nil
}

func Test(host, key string) error {
	cb := getBreaker()

	return cb.Execute(func() error {
		return doTest(host, key)
	})
}

func doTest(host, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := retry.Config{
		MaxAttempts:  2,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Multiplier:   2.0,
	}

	return retry.DoNoResult(ctx, cfg, func() error {
		if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
			host = "http://" + host
		}

		if !strings.HasSuffix(host, "/") {
			host += "/"
		}

		u, err := url.Parse(host + "api")
		if err != nil {
			return err
		}

		q := u.Query()
		q.Set("apikey", key)
		q.Set("t", "caps")
		u.RawQuery = q.Encode()

		resp, err := http.Get(u.String())
		if err != nil {
			return err
		}

		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status: %s", resp.Status)
		}

		var probe struct {
			XMLName     xml.Name
			Code        string `xml:"code,attr"`
			Description string `xml:"description,attr"`
		}

		if err := xml.NewDecoder(resp.Body).Decode(&probe); err != nil {
			return fmt.Errorf("invalid xml response: %v", err)
		}

		if probe.XMLName.Local == "error" {
			msg := probe.Description
			if msg == "" {
				msg = probe.Code
			}

			return fmt.Errorf("api error: %s", msg)
		}

		if probe.XMLName.Local != "caps" {
			return fmt.Errorf("unexpected xml root: %s", probe.XMLName.Local)
		}

		return nil
	})
}

func parseDate(dateStr string) time.Time {
	t, err := time.Parse(time.RFC1123, dateStr)
	if err != nil {
		t, err = time.Parse(time.RFC1123Z, dateStr)
		if err != nil {
			return time.Now()
		}
	}

	return t
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cCiB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func extractHash(magnet string) string {
	if strings.HasPrefix(magnet, "magnet:?") {
		u, err := url.Parse(magnet)
		if err == nil {
			xt := u.Query().Get("xt")
			if after, ok := strings.CutPrefix(xt, "urn:btih:"); ok {
				return after
			}
		}
	}

	return ""
}

func GetCircuitBreakerMetrics() map[string]any {
	return getBreaker().Metrics()
}
