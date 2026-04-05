package settings

import (
	"encoding/json"
)

type Viewed struct {
	Hash      string `json:"hash"`
	FileIndex int    `json:"file_index"`
}

func SetViewed(vv *Viewed) {
	if tdb == nil || vv == nil {
		return
	}

	var indexes map[int]struct{}

	var err error

	buf := tdb.Get("Viewed", vv.Hash)
	if len(buf) == 0 {
		indexes = make(map[int]struct{})
		indexes[vv.FileIndex] = struct{}{}

		buf, err = json.Marshal(indexes)
		if err == nil {
			tdb.Set("Viewed", vv.Hash, buf)
		}
	} else {
		err = json.Unmarshal(buf, &indexes)
		if err == nil {
			indexes[vv.FileIndex] = struct{}{}

			buf, err = json.Marshal(indexes)
			if err == nil {
				tdb.Set("Viewed", vv.Hash, buf)
			}
		}
	}
}

func RemViewed(vv *Viewed) {
	if tdb == nil || vv == nil {
		return
	}

	buf := tdb.Get("Viewed", vv.Hash)

	var indices map[int]struct{}

	err := json.Unmarshal(buf, &indices)
	if err == nil {
		if vv.FileIndex != -1 {
			delete(indices, vv.FileIndex)

			buf, err = json.Marshal(indices)
			if err == nil {
				tdb.Set("Viewed", vv.Hash, buf)
			}
		} else {
			tdb.Rem("Viewed", vv.Hash)
		}
	}
}

func ListViewed(hash string) []*Viewed {
	if tdb == nil {
		return []*Viewed{}
	}

	buf := tdb.Get("Viewed", hash)
	if len(buf) == 0 {
		return []*Viewed{}
	}

	var indices map[int]struct{}
	if err := json.Unmarshal(buf, &indices); err == nil {
		var ret []*Viewed
		for i := range indices {
			ret = append(ret, &Viewed{hash, i})
		}

		return ret
	}

	return []*Viewed{}
}
