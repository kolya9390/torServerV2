package torr

import (
	"io"
	"time"

	"server/log"
	sets "server/settings"
)

// RuntimeController encapsulates BTServer-specific runtime control operations.
// It is intentionally separate from TorrentService so playback/data access and
// process/runtime mutation are not mixed into one responsibility.
type RuntimeController interface {
	WriteStatus(io.Writer)
	ApplySettings(*sets.BTSets)
	ResetDefaultSettings()
	Shutdown()
	ActivePlaybackTorrents() int
}

type btRuntimeController struct {
	bt *BTServer
}

type noopRuntimeController struct{}

func NewRuntimeControllerWithBT(bt *BTServer) RuntimeController {
	return btRuntimeController{bt: bt}
}

func NewNoopRuntimeController() RuntimeController {
	return noopRuntimeController{}
}

func (c btRuntimeController) WriteStatus(w io.Writer) {
	if c.bt == nil || c.bt.client == nil {
		return
	}

	c.bt.client.WriteStatus(w)
}

func (c btRuntimeController) ApplySettings(set *sets.BTSets) {
	if c.bt == nil {
		return
	}

	if sets.IsReadOnlyMode() {
		log.TLogln("API SetSettings: Read-only DB mode!")

		return
	}

	c.setCurrentSettings(set)

	if hasActivePlaybackBT(c.bt) {
		log.TLogln("SetSettings: skip disruptive reconnect while playback is active")

		return
	}

	log.TLogln("drop all torrents")
	dropAllTorrentBT(c.bt)
	time.Sleep(time.Second)
	log.TLogln("disconect")
	c.bt.Disconnect()
	log.TLogln("connect")

	if err := c.bt.Connect(); err != nil {
		log.TLogln("Connect error:", err)
	}

	time.Sleep(time.Second)
	log.TLogln("end set settings")
}

func (c btRuntimeController) ResetDefaultSettings() {
	if c.bt == nil {
		return
	}

	if sets.IsReadOnlyMode() {
		log.TLogln("API SetDefSettings: Read-only DB mode!")

		return
	}

	sets.SetDefaultConfig()

	if hasActivePlaybackBT(c.bt) {
		log.TLogln("SetDefSettings: skip disruptive reconnect while playback is active")

		return
	}

	log.TLogln("drop all torrents")
	dropAllTorrentBT(c.bt)
	time.Sleep(time.Second)
	log.TLogln("disconect")
	c.bt.Disconnect()
	log.TLogln("connect")

	if err := c.bt.Connect(); err != nil {
		log.TLogln("Connect error:", err)
	}

	time.Sleep(time.Second)
	log.TLogln("end set default settings")
}

func (c btRuntimeController) Shutdown() {
	if c.bt == nil {
		return
	}

	c.bt.Disconnect()
	sets.CloseDB()
	log.TLogln("Received shutdown. Quit")
}

func (c btRuntimeController) ActivePlaybackTorrents() int {
	if c.bt == nil {
		return 0
	}

	return c.bt.ActivePlaybackTorrents()
}

func (noopRuntimeController) WriteStatus(io.Writer)       {}
func (noopRuntimeController) ApplySettings(*sets.BTSets)  {}
func (noopRuntimeController) ResetDefaultSettings()       {}
func (noopRuntimeController) Shutdown()                   {}
func (noopRuntimeController) ActivePlaybackTorrents() int { return 0 }

func (c btRuntimeController) setCurrentSettings(set *sets.BTSets) {
	if set == nil {
		return
	}

	if c.bt != nil && c.bt.deps.settingsProvider != nil {
		c.bt.deps.settingsProvider.Set(set)
	}
}

func dropAllTorrentBT(bt *BTServer) {
	for _, torr := range bt.ListTorrents() {
		torr.drop()
		select {
		case <-torr.lifecycle.closed:
		case <-time.After(5 * time.Second):
			log.TLogln("dropAllTorrent: timeout waiting for torrent close", torr.Hash().HexString())
		}
	}
}

func hasActivePlaybackBT(bt *BTServer) bool {
	if GetActiveStreams() > 0 {
		return true
	}

	for _, torr := range bt.ListTorrents() {
		if torr != nil && torr.cache != nil && torr.cache.Readers() > 0 {
			return true
		}
	}

	return false
}
