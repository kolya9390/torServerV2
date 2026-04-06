package utils

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"runtime"
	"server/log"
	"server/torrshash"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func ParseFromBytes(data []byte) (*torrent.TorrentSpec, error) {
	minfo, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return torrent.TorrentSpecFromMetaInfo(minfo), nil
}

func ParseFile(file multipart.File) (*torrent.TorrentSpec, error) {
	minfo, err := metainfo.Load(file)
	if err != nil {
		return nil, err
	}

	return torrent.TorrentSpecFromMetaInfo(minfo), nil
}

func ParseLink(link string) (*torrent.TorrentSpec, error) {
	urlLink, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(urlLink.Scheme) {
	case "magnet":
		return fromMagnet(urlLink.String())
	case "http", "https":
		return fromHttp(urlLink.String())
	case "":
		return fromMagnet("magnet:?xt=urn:btih:" + urlLink.Path)
	case "file":
		return fromFile(urlLink.Path)
	default:
		err = fmt.Errorf("unknown scheme %q in link %q", urlLink.Scheme, urlLink.String())
	}

	return nil, err
}

func fromMagnet(link string) (*torrent.TorrentSpec, error) {
	spec, err := torrent.TorrentSpecFromMagnetUri(link)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func ParseTorrsHash(token string) (*torrent.TorrentSpec, *torrshash.TorrsHash, error) {
	token = strings.TrimPrefix(token, "torrs://")

	th, err := torrshash.Unpack(token)
	if err != nil {
		return nil, nil, err
	}

	var trackers [][]string
	if len(th.Trackers()) > 0 {
		trackers = [][]string{th.Trackers()}
	}

	return &torrent.TorrentSpec{
		AddTorrentOpts: torrent.AddTorrentOpts{
			InfoHash: metainfo.NewHashFromHex(th.Hash),
		},
		Trackers:    trackers,
		DisplayName: th.Title(),
	}, th, nil
}

func fromHttp(link string) (*torrent.TorrentSpec, error) {
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}

	client := new(http.Client)
	client.Timeout = 60 * time.Second

	req.Header.Set("User-Agent", "DWL/1.1.1 (Torrent)")

	resp, err := client.Do(req)
	if er, ok := err.(*url.Error); ok {
		if strings.HasPrefix(er.URL, "magnet:") {
			return fromMagnet(er.URL)
		}
	}

	if err != nil {
		return nil, err
	}

	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.TLogln("error closing torrent response body:", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	minfo, err := metainfo.Load(resp.Body)
	if err != nil {
		return nil, err
	}

	return torrent.TorrentSpecFromMetaInfo(minfo), nil
}

func fromFile(path string) (*torrent.TorrentSpec, error) {
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") {
		path = strings.TrimPrefix(path, "/")
	}

	minfo, err := metainfo.LoadFromFile(path)
	if err != nil {
		return nil, err
	}

	return torrent.TorrentSpecFromMetaInfo(minfo), nil
}
