package torrentparse

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// ParseLink parses magnet/hash/http/file torrent links into torrent spec.
func ParseLink(link string) (*torrent.TorrentSpec, error) {
	urlLink, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(urlLink.Scheme) {
	case "magnet":
		return fromMagnet(urlLink.String())
	case "http", "https":
		return fromHTTP(urlLink.String())
	case "":
		return fromMagnet("magnet:?xt=urn:btih:" + urlLink.Path)
	case "file":
		return fromFile(urlLink.Path)
	default:
		return nil, fmt.Errorf("unknown scheme %q in link %q", urlLink.Scheme, urlLink.String())
	}
}

func fromMagnet(link string) (*torrent.TorrentSpec, error) {
	spec, err := torrent.TorrentSpecFromMagnetUri(link)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func fromHTTP(link string) (*torrent.TorrentSpec, error) {
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 60 * time.Second}

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
		_ = resp.Body.Close()
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

// ParseFromBytes parses .torrent payload bytes into torrent spec.
func ParseFromBytes(data []byte) (*torrent.TorrentSpec, error) {
	minfo, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return torrent.TorrentSpecFromMetaInfo(minfo), nil
}
