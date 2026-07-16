package torrstor

import (
	"sort"
	"sync"
)

const maxRequestStrategyCapacityItems = 16

// RequestStrategyCapacityItem describes one anonymous storage scheduling
// bound. It deliberately contains no torrent or client identity.
type RequestStrategyCapacityItem struct {
	ContractEnabled                 bool   `json:"contract_enabled"`
	CapacityCapped                  bool   `json:"capacity_capped"`
	CapacityBytes                   int64  `json:"capacity_bytes"`
	PieceLengthBytes                int64  `json:"piece_length_bytes"`
	TorrentPieceCount               int    `json:"torrent_piece_count"`
	BoundedRequestablePieceEstimate int64  `json:"bounded_requestable_piece_estimate"`
	Fault                           string `json:"fault,omitempty"`
}

// RequestStrategyCapacityDiagnostics is a bounded, privacy-safe view of the
// storage contract used by anacrolix request ordering.
type RequestStrategyCapacityDiagnostics struct {
	Status                          string                        `json:"status"`
	Interpretation                  string                        `json:"interpretation"`
	ActiveCaches                    int                           `json:"active_caches"`
	CappedCaches                    int                           `json:"capped_caches"`
	UncappedCaches                  int                           `json:"uncapped_caches"`
	InvalidCaches                   int                           `json:"invalid_caches"`
	OmittedCaches                   int                           `json:"omitted_caches"`
	BoundedRequestablePieceEstimate int64                         `json:"bounded_requestable_piece_estimate"`
	Caches                          []RequestStrategyCapacityItem `json:"caches"`
}

var requestStrategyCapacityRegistry = struct {
	mu     sync.RWMutex
	caches map[*Cache]bool
}{
	caches: make(map[*Cache]bool),
}

func (c *Cache) registerRequestStrategyCapacityDiagnostics(contractEnabled bool) {
	if c == nil || !c.debugMetricsEnabled() {
		return
	}

	requestStrategyCapacityRegistry.mu.Lock()
	requestStrategyCapacityRegistry.caches[c] = contractEnabled
	requestStrategyCapacityRegistry.mu.Unlock()
}

func (c *Cache) unregisterRequestStrategyCapacityDiagnostics() {
	if c == nil {
		return
	}

	requestStrategyCapacityRegistry.mu.Lock()
	delete(requestStrategyCapacityRegistry.caches, c)
	requestStrategyCapacityRegistry.mu.Unlock()
}

// SnapshotRequestStrategyCapacityDiagnostics computes diagnostics only when
// the debug endpoint requests them. Release mode never registers caches here.
func SnapshotRequestStrategyCapacityDiagnostics() RequestStrategyCapacityDiagnostics {
	entries := snapshotRequestStrategyCapacityEntries()
	items := make([]RequestStrategyCapacityItem, 0, len(entries))

	for _, entry := range entries {
		if entry.cache == nil || entry.cache.isClosed.Load() {
			continue
		}

		items = append(items, requestStrategyCapacityItem(entry.cache, entry.contractEnabled))
	}

	sort.Slice(items, func(first, second int) bool {
		if items[first].Fault != items[second].Fault {
			return items[first].Fault > items[second].Fault
		}

		if items[first].CapacityBytes != items[second].CapacityBytes {
			return items[first].CapacityBytes < items[second].CapacityBytes
		}

		if items[first].PieceLengthBytes != items[second].PieceLengthBytes {
			return items[first].PieceLengthBytes < items[second].PieceLengthBytes
		}

		return items[first].TorrentPieceCount < items[second].TorrentPieceCount
	})

	return summarizeRequestStrategyCapacityItems(items)
}

type requestStrategyCapacityEntry struct {
	cache           *Cache
	contractEnabled bool
}

func snapshotRequestStrategyCapacityEntries() []requestStrategyCapacityEntry {
	requestStrategyCapacityRegistry.mu.RLock()
	defer requestStrategyCapacityRegistry.mu.RUnlock()

	entries := make([]requestStrategyCapacityEntry, 0, len(requestStrategyCapacityRegistry.caches))
	for cache, enabled := range requestStrategyCapacityRegistry.caches {
		entries = append(entries, requestStrategyCapacityEntry{
			cache:           cache,
			contractEnabled: enabled,
		})
	}

	return entries
}

func requestStrategyCapacityItem(cache *Cache, contractEnabled bool) RequestStrategyCapacityItem {
	capacity := cache.GetCapacity()
	capped := false

	if contractEnabled {
		capacity, capped = cache.requestStrategyCapacity()
	}

	item := RequestStrategyCapacityItem{
		ContractEnabled:   contractEnabled,
		CapacityCapped:    capped,
		CapacityBytes:     capacity,
		PieceLengthBytes:  cache.pieceLength,
		TorrentPieceCount: cache.pieceCount,
	}

	switch {
	case !contractEnabled || !capped:
		item.Fault = "missing_capacity_contract"
	case capacity <= 0 || cache.pieceLength <= 0 || cache.pieceCount <= 0:
		item.Fault = "invalid_capacity_contract"
	default:
		item.BoundedRequestablePieceEstimate = boundedRequestablePieceEstimate(
			capacity,
			cache.pieceLength,
			cache.pieceCount,
		)
	}

	return item
}

func boundedRequestablePieceEstimate(capacity, pieceLength int64, pieceCount int) int64 {
	if capacity <= 0 || pieceLength <= 0 || pieceCount <= 0 {
		return 0
	}

	estimate := capacity / pieceLength
	if estimate > int64(pieceCount) {
		return int64(pieceCount)
	}

	return estimate
}

func summarizeRequestStrategyCapacityItems(items []RequestStrategyCapacityItem) RequestStrategyCapacityDiagnostics {
	diagnostics := RequestStrategyCapacityDiagnostics{
		Status:         "inactive",
		Interpretation: "no debug-registered torrent storage is active",
		ActiveCaches:   len(items),
		Caches:         items,
	}

	for _, item := range items {
		diagnostics.BoundedRequestablePieceEstimate += item.BoundedRequestablePieceEstimate

		switch item.Fault {
		case "missing_capacity_contract":
			diagnostics.UncappedCaches++
		case "invalid_capacity_contract":
			diagnostics.InvalidCaches++
		default:
			diagnostics.CappedCaches++
		}
	}

	if len(diagnostics.Caches) > maxRequestStrategyCapacityItems {
		diagnostics.OmittedCaches = len(diagnostics.Caches) - maxRequestStrategyCapacityItems
		diagnostics.Caches = diagnostics.Caches[:maxRequestStrategyCapacityItems]
	}

	switch {
	case diagnostics.UncappedCaches > 0:
		diagnostics.Status = "fault_missing_capacity"
		diagnostics.Interpretation = "active torrent storage is uncapped; request ordering can scan the full torrent"
	case diagnostics.InvalidCaches > 0:
		diagnostics.Status = "fault_invalid_capacity"
		diagnostics.Interpretation = "active torrent storage reported an invalid scheduling capacity"
	case diagnostics.ActiveCaches > 0:
		diagnostics.Status = "ok"
		diagnostics.Interpretation = "all active torrent storage is bounded for request ordering"
	}

	return diagnostics
}
