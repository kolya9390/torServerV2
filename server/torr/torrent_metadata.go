package torr

type TorrentMetadata struct {
	Title    string
	Poster   string
	Category string
	Data     string
}

func (t *Torrent) Metadata() TorrentMetadata {
	if t == nil {
		return TorrentMetadata{}
	}

	return TorrentMetadata{
		Title:    t.Title,
		Poster:   t.Poster,
		Category: t.Category,
		Data:     t.Data,
	}
}

func (t *Torrent) SetMetadata(meta TorrentMetadata) {
	if t == nil {
		return
	}

	t.Title = meta.Title
	t.Poster = meta.Poster
	t.Category = meta.Category
	t.Data = meta.Data
}

func (t *Torrent) FillMissingMetadata(meta TorrentMetadata) {
	if t == nil {
		return
	}

	if t.Title == "" {
		t.Title = meta.Title
	}

	if t.Poster == "" {
		t.Poster = meta.Poster
	}

	if t.Category == "" {
		t.Category = meta.Category
	}

	if t.Data == "" {
		t.Data = meta.Data
	}
}

func (t *Torrent) EnsureTitleFromInfo() {
	if t == nil {
		return
	}

	if t.Title == "" && t.Torrent != nil && t.Info() != nil {
		t.Title = t.Info().Name
	}
}
