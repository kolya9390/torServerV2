package torr

import "testing"

func TestComputeAdaptiveReadaheadIncreasesOnLowBufferAndLowRatio(t *testing.T) {
	in := adaptiveRAInput{
		pieceLength: 1 << 20,
		cacheCap:    256 << 20,
		readers:     1,
		downloadBps: 2 * 1024 * 1024,
		bitrate:     "32000000", // 4MB/s
		buffered:    16 << 20,   // low health
		currentRA:   8 << 20,
		minRA:       4 << 20,
		maxRA:       64 << 20,
	}
	got := computeAdaptiveReadahead(in)
	if got <= in.currentRA {
		t.Fatalf("expected readahead to grow on low health/ratio: current=%d got=%d", in.currentRA, got)
	}
}

func TestComputeAdaptiveReadaheadShrinksWhenHealthyAndFast(t *testing.T) {
	in := adaptiveRAInput{
		pieceLength: 1 << 20,
		cacheCap:    256 << 20,
		readers:     1,
		downloadBps: 24 * 1024 * 1024,
		bitrate:     "8000000", // 1MB/s
		buffered:    220 << 20, // high health
		currentRA:   48 << 20,
		minRA:       4 << 20,
		maxRA:       64 << 20,
	}
	got := computeAdaptiveReadahead(in)
	if got >= in.currentRA {
		t.Fatalf("expected readahead to shrink on healthy/fast link: current=%d got=%d", in.currentRA, got)
	}
}

func TestComputeAdaptiveReadaheadRespectsBounds(t *testing.T) {
	in := adaptiveRAInput{
		pieceLength: 2 << 20,
		cacheCap:    64 << 20,
		readers:     2,
		downloadBps: 128 * 1024,
		bitrate:     "64000000",
		buffered:    1 << 20,
		currentRA:   0,
		minRA:       4 << 20,
		maxRA:       40 << 20,
	}
	got := computeAdaptiveReadahead(in)
	if got < in.minRA {
		t.Fatalf("readahead below min bound: got=%d min=%d", got, in.minRA)
	}
	perReaderCap := in.cacheCap / int64(in.readers)
	if got > perReaderCap {
		t.Fatalf("readahead exceeds per-reader cap: got=%d cap=%d", got, perReaderCap)
	}
	if got%(2<<20) != 0 {
		t.Fatalf("readahead must align to piece size, got=%d", got)
	}
}

func TestParseBitRate(t *testing.T) {
	bps, ok := parseBitRate("16000000")
	if !ok {
		t.Fatalf("expected parseBitRate to parse numeric bitrate")
	}
	if bps != 2000000 {
		t.Fatalf("unexpected bitrate conversion, got=%f", bps)
	}
}
