package torr

import (
	"net"

	"github.com/anacrolix/torrent"

	"server/log"
	"server/settings"
	"server/torr/storage"
)

type BTServer struct {
	config *torrent.ClientConfig
	client *torrent.Client

	storage storage.Storage

	registry *btTorrentRegistry
	deps     btServerDeps
}

type btServerDeps struct {
	settingsProvider settings.SettingsProvider
	getArgs          func() *settings.ExecArgs
	runtimeState     func() settings.RuntimeState
	dbStore          TorrentDBStore
}

var privateIPBlocks []*net.IPNet

var initPrivateIPBlocks = []string{
	"127.0.0.0/8",    // IPv4 loopback
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
	"169.254.0.0/16", // RFC3927 link-local
	"::1/128",        // IPv6 loopback
	"fe80::/10",      // IPv6 link-local
	"fc00::/7",       // IPv6 unique local addr
}

func init() {
	for _, cidr := range initPrivateIPBlocks {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			log.TLogln("Warning: invalid CIDR, skipping", cidr, err)

			continue
		}

		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func NewBTS() *BTServer {
	bts := new(BTServer)
	bts.registry = newBTTorrentRegistry()
	bts.deps = defaultBTServerDeps()

	return bts
}

func NewBTSWithProvidersRuntimeAndDB(settingsProvider settings.SettingsProvider, argsProvider settings.ArgsProvider, runtimeState func() settings.RuntimeState, dbStore TorrentDBStore) *BTServer {
	var deps btServerDeps
	deps.settingsProvider = settingsProvider
	if argsProvider != nil {
		deps.getArgs = argsProvider.Get
	}
	deps.runtimeState = runtimeState
	deps.dbStore = dbStore

	bts := &BTServer{
		registry: newBTTorrentRegistry(),
		deps:     resolveBTServerDeps(deps),
	}

	return bts
}

func defaultBTServerDeps() btServerDeps {
	return btServerDeps{
		settingsProvider: settings.DefaultSettingsProvider,
		getArgs:          settings.DefaultArgsProvider.Get,
		runtimeState:     settings.GetRuntimeState,
		dbStore:          NewSettingsTorrentDBStore(),
	}
}

func resolveBTServerDeps(deps btServerDeps) btServerDeps {
	if deps.settingsProvider == nil {
		deps.settingsProvider = settings.NewNoopSettingsProvider()
	}

	if deps.getArgs == nil {
		deps.getArgs = settings.NewNoopArgsProvider().Get
	}

	if deps.runtimeState == nil {
		deps.runtimeState = func() settings.RuntimeState { return settings.RuntimeState{} }
	}

	if deps.dbStore == nil {
		deps.dbStore = NewNoopTorrentDBStore()
	}

	return deps
}

func (bt *BTServer) currentSettings() *settings.BTSets {
	if bt != nil && bt.deps.settingsProvider != nil {
		return bt.deps.settingsProvider.Get()
	}

	return nil
}

func (bt *BTServer) currentArgs() *settings.ExecArgs {
	if bt != nil && bt.deps.getArgs != nil {
		return bt.deps.getArgs()
	}

	return nil
}

func (bt *BTServer) currentRuntimeState() settings.RuntimeState {
	if bt != nil && bt.deps.runtimeState != nil {
		return bt.deps.runtimeState()
	}

	return settings.RuntimeState{}
}

func (bt *BTServer) currentDBStore() TorrentDBStore {
	if bt != nil && bt.deps.dbStore != nil {
		return bt.deps.dbStore
	}

	return NewNoopTorrentDBStore()
}
