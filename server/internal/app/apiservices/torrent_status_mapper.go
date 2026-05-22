package apiservices

import (
	"server/internal/app/contracts"
	"server/torr/state"
)

func mapTorrentState(src state.TorrentStat) contracts.TorrentState {
	return contracts.TorrentState(src)
}

func mapTorrentStatus(src *state.TorrentStatus) *contracts.TorrentStatus {
	if src == nil {
		return nil
	}

	return &contracts.TorrentStatus{
		Title:               src.Title,
		Category:            src.Category,
		Poster:              src.Poster,
		Data:                src.Data,
		Timestamp:           src.Timestamp,
		Name:                src.Name,
		Hash:                src.Hash,
		TorrsHash:           src.TorrsHash,
		Stat:                mapTorrentState(src.Stat),
		StatString:          src.StatString,
		LoadedSize:          src.LoadedSize,
		TorrentSize:         src.TorrentSize,
		PreloadedBytes:      src.PreloadedBytes,
		PreloadSize:         src.PreloadSize,
		DownloadSpeed:       src.DownloadSpeed,
		UploadSpeed:         src.UploadSpeed,
		TotalPeers:          src.TotalPeers,
		PendingPeers:        src.PendingPeers,
		ActivePeers:         src.ActivePeers,
		ConnectedSeeders:    src.ConnectedSeeders,
		HalfOpenPeers:       src.HalfOpenPeers,
		BytesWritten:        src.BytesWritten,
		BytesWrittenData:    src.BytesWrittenData,
		BytesRead:           src.BytesRead,
		BytesReadData:       src.BytesReadData,
		BytesReadUsefulData: src.BytesReadUsefulData,
		ChunksWritten:       src.ChunksWritten,
		ChunksRead:          src.ChunksRead,
		ChunksReadUseful:    src.ChunksReadUseful,
		ChunksReadWasted:    src.ChunksReadWasted,
		PiecesDirtiedGood:   src.PiecesDirtiedGood,
		PiecesDirtiedBad:    src.PiecesDirtiedBad,
		DurationSeconds:     src.DurationSeconds,
		BitRate:             src.BitRate,
		FileStats:           mapTorrentFiles(src.FileStats),
	}
}

func mapTorrentFiles(src []*state.TorrentFileStat) []*contracts.TorrentFile {
	if len(src) == 0 {
		return nil
	}

	mapped := make([]*contracts.TorrentFile, 0, len(src))

	for _, file := range src {
		if file == nil {
			continue
		}

		mapped = append(mapped, &contracts.TorrentFile{
			ID:     file.ID,
			Path:   file.Path,
			Length: file.Length,
		})
	}

	return mapped
}
