package log

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestSetLevelUpdatesInitializedLogger(t *testing.T) {
	if err := SetLevel("info"); err != nil {
		t.Fatalf("SetLevel(info): %v", err)
	}

	Init("", "")

	if logger == nil {
		t.Fatal("expected logger to be initialized")
	}

	if logger.Desugar().Core().Enabled(zapcore.DebugLevel) {
		t.Fatal("debug logging should be disabled at info level")
	}

	if err := SetLevel("debug"); err != nil {
		t.Fatalf("SetLevel(debug): %v", err)
	}

	if !logger.Desugar().Core().Enabled(zapcore.DebugLevel) {
		t.Fatal("debug logging should be enabled after SetLevel(debug)")
	}

	if err := SetLevel("info"); err != nil {
		t.Fatalf("SetLevel(info): %v", err)
	}
}

func TestDebugHelpersRespectLevel(t *testing.T) {
	if err := SetLevel("info"); err != nil {
		t.Fatalf("SetLevel(info): %v", err)
	}

	Init("", "")

	if IsDebugEnabled() {
		t.Fatal("debug helper should report disabled at info level")
	}

	if err := SetLevel("debug"); err != nil {
		t.Fatalf("SetLevel(debug): %v", err)
	}

	if !IsDebugEnabled() {
		t.Fatal("debug helper should report enabled at debug level")
	}

	DebugSampled("test.debug_helpers", 2, "sampled debug message", "ok", true)

	if err := SetLevel("info"); err != nil {
		t.Fatalf("SetLevel(info): %v", err)
	}
}
