package torr

import (
	"strconv"
	"strings"
)

type adaptiveRAInput struct {
	pieceLength int64
	cacheCap    int64
	readers     int
	downloadBps float64
	bitrate     string
	buffered    int64
	currentRA   int64
	minRA       int64
	maxRA       int64
}

func computeAdaptiveReadahead(in adaptiveRAInput) int64 {
	if in.pieceLength <= 0 {
		return 0
	}
	if in.readers <= 0 {
		in.readers = 1
	}

	minRA := in.minRA
	maxRA := in.maxRA
	if minRA <= 0 {
		minRA = 4 << 20
	}
	if maxRA <= 0 {
		maxRA = 64 << 20
	}
	if minRA < in.pieceLength {
		minRA = in.pieceLength
	}
	perReaderCap := in.cacheCap / int64(in.readers)
	if perReaderCap > 0 && maxRA > perReaderCap {
		maxRA = perReaderCap
	}
	if maxRA < minRA {
		maxRA = minRA
	}

	requiredBps := inferRequiredRateBps(in.bitrate, in.downloadBps)
	ratio := in.downloadBps / requiredBps

	targetSec := 30.0
	switch {
	case ratio < 0.75:
		targetSec = 55
	case ratio < 1.0:
		targetSec = 40
	case ratio > 2.5:
		targetSec = 12
	case ratio > 1.75:
		targetSec = 18
	}

	targetBuffer := requiredBps * targetSec
	health := 0.0
	if in.cacheCap > 0 {
		health = float64(in.buffered) / float64(in.cacheCap)
	}
	switch {
	case health < 0.20:
		targetBuffer *= 1.30
	case health > 0.80:
		targetBuffer *= 0.75
	}

	targetRA := int64(targetBuffer / float64(in.readers))
	targetRA = clampInt64(targetRA, minRA, maxRA)
	targetRA = alignToPiece(targetRA, in.pieceLength)

	current := in.currentRA
	if current <= 0 {
		return targetRA
	}
	step := in.pieceLength
	if step < 1<<20 {
		step = 1 << 20
	}
	if targetRA > current+step {
		targetRA = current + step
	} else if targetRA < current-step {
		targetRA = current - step
	}
	return clampInt64(alignToPiece(targetRA, in.pieceLength), minRA, maxRA)
}

func inferRequiredRateBps(bitRate string, downloadBps float64) float64 {
	if bps, ok := parseBitRate(bitRate); ok && bps > 0 {
		// Keep network headroom above pure media bitrate.
		return bps * 1.15
	}
	// Fallback for unknown bitrate.
	fallback := downloadBps * 0.60
	if fallback < 800*1024 {
		fallback = 800 * 1024
	}
	return fallback
}

func parseBitRate(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "n/a") {
		return 0, false
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	// ffprobe bit_rate is bits/sec.
	return n / 8.0, true
}

func alignToPiece(v, pieceLen int64) int64 {
	if pieceLen <= 0 || v <= 0 {
		return v
	}
	if v%pieceLen == 0 {
		return v
	}
	return ((v / pieceLen) + 1) * pieceLen
}

func clampInt64(v, minV, maxV int64) int64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
