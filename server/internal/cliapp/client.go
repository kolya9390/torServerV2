package cliapp

import (
	"context"

	"server/internal/apiclient"
)

type apiClient = apiclient.Client

type baseURLAPI interface {
	BaseURL() string
}

type statusAPI interface {
	baseURLAPI
	Version(context.Context) (apiclient.Version, error)
	Readiness(context.Context) (apiclient.Readiness, error)
}

type torrentReadAPI interface {
	baseURLAPI
	ListTorrents(context.Context) ([]apiclient.Torrent, error)
	GetTorrent(context.Context, string) (apiclient.Torrent, error)
}

type torrentAddAPI interface {
	AddTorrent(context.Context, apiclient.AddTorrentRequest) (apiclient.Torrent, error)
	UploadTorrent(context.Context, apiclient.UploadTorrentRequest) (apiclient.Torrent, error)
}

type torrentMutationAPI interface {
	torrentReadAPI
	RemoveTorrent(context.Context, string) error
	DropTorrent(context.Context, string) error
}

type torrentWipeAPI interface {
	WipeTorrents(context.Context) error
}

type settingsAPI interface {
	GetSettings(context.Context) (apiclient.Settings, error)
	SetSettings(context.Context, apiclient.SettingsPatch) error
	ResetSettings(context.Context) error
}

type authAPI interface {
	ListUsers(context.Context) (apiclient.Users, error)
	AddUser(context.Context, string, string) error
	RemoveUser(context.Context, string) error
}

type shutdownTokenAPI interface {
	ShutdownTokenStatus(context.Context) (apiclient.ShutdownTokenStatus, error)
	GenerateShutdownToken(context.Context) (apiclient.GeneratedShutdownToken, error)
	SetShutdownToken(context.Context, string) error
}

type shutdownAPI interface {
	Shutdown(context.Context, apiclient.ShutdownRequest) error
}
