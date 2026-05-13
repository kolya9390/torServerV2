package torr

// RuntimeSignals exposes read-only runtime state needed by higher layers.
// It intentionally stays separate from RuntimeController so observation and
// mutation do not collapse into one interface.
type RuntimeSignals interface {
	HasRuntimeBackend() bool
	ActivePlaybackTorrents() int
	ActiveStreams() int32
}

type btRuntimeSignals struct {
	bt *BTServer
}

type noopRuntimeSignals struct{}

func NewRuntimeSignalsWithBT(bt *BTServer) RuntimeSignals {
	return btRuntimeSignals{bt: bt}
}

func NewNoopRuntimeSignals() RuntimeSignals {
	return noopRuntimeSignals{}
}

func (s btRuntimeSignals) HasRuntimeBackend() bool {
	return s.bt != nil
}

func (s btRuntimeSignals) ActivePlaybackTorrents() int {
	if s.bt == nil {
		return 0
	}

	return s.bt.ActivePlaybackTorrents()
}

func (s btRuntimeSignals) ActiveStreams() int32 {
	return GetActiveStreams()
}

func (noopRuntimeSignals) HasRuntimeBackend() bool {
	return false
}

func (noopRuntimeSignals) ActivePlaybackTorrents() int {
	return 0
}

func (noopRuntimeSignals) ActiveStreams() int32 {
	return 0
}
