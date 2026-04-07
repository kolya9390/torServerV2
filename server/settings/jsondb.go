package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"server/log"
)

type JSONDB struct {
	Path              string
	filenameDelimiter string
	filenameExtension string
	fileMode          fs.FileMode
	xPathDelimeter    string
}

var globalJSONDB TorrServerDB
var globalJSONDBMu sync.Mutex
var jsonDBLocks = make(map[string]*sync.Mutex)
var jsonDBLocksMutex sync.Mutex

func NewJSONDB() TorrServerDB {
	globalJSONDBMu.Lock()
	defer globalJSONDBMu.Unlock()

	if globalJSONDB != nil {
		return globalJSONDB
	}

	globalJSONDB = &JSONDB{
		Path:              Path,
		filenameDelimiter: ".",
		filenameExtension: ".json",
		fileMode:          fs.FileMode(0o666),
		xPathDelimeter:    "/",
	}

	return globalJSONDB
}

func (v *JSONDB) CloseDB() {
	// Not necessary
}

func (v *JSONDB) Set(xPath, name string, value []byte) {
	var err error = nil

	jsonObj := map[string]any{}
	if err := json.Unmarshal(value, &jsonObj); err == nil {
		if filename, err := v.xPathToFilename(xPath); err == nil {
			v.lock(filename)
			defer v.unlock(filename)

			if root, err := v.readJSONFileAsMap(filename); err == nil {
				root[name] = jsonObj
				if err = v.writeMapAsJSONFile(filename, root); err == nil {
					return
				}
			}
		}
	}

	v.log(fmt.Sprintf("Set: error writing entry %s->%s", xPath, name), err)
}

func (v *JSONDB) Get(xPath, name string) []byte {
	var err error = nil
	if filename, err := v.xPathToFilename(xPath); err == nil {
		v.lock(filename)
		defer v.unlock(filename)

		if root, err := v.readJSONFileAsMap(filename); err == nil {
			if jsonData, ok := root[name]; ok {
				if byteData, err := json.Marshal(jsonData); err == nil {
					// Return a copy to be safe
					data := make([]byte, len(byteData))
					copy(data, byteData)

					return data
				}
			} else {
				// We assume this is not 'error' but 'no entry' which is normal
				return nil
			}
		}
	}

	v.log(fmt.Sprintf("Get: error reading entry %s->%s", xPath, name), err)

	return nil
}

func (v *JSONDB) List(xPath string) []string {
	var err error = nil
	if filename, err := v.xPathToFilename(xPath); err == nil {
		v.lock(filename)
		defer v.unlock(filename)

		if root, err := v.readJSONFileAsMap(filename); err == nil {
			nameList := make([]string, 0, len(root))
			for k := range root {
				nameList = append(nameList, k)
			}

			return nameList
		}
	}

	v.log("List: error reading entries in xPath "+xPath, err)

	return nil
}

func (v *JSONDB) Rem(xPath, name string) {
	var err error = nil
	if filename, err := v.xPathToFilename(xPath); err == nil {
		v.lock(filename)
		defer v.unlock(filename)

		if root, err := v.readJSONFileAsMap(filename); err == nil {
			delete(root, name)

			if err = v.writeMapAsJSONFile(filename, root); err == nil {
				return
			}
		}
	}

	v.log(fmt.Sprintf("Rem: error removing entry %s->%s", xPath, name), err)
}

func (v *JSONDB) Clear(xPath string) {
	filename, err := v.xPathToFilename(xPath)
	if err != nil {
		v.log(fmt.Sprintf("Clear: error converting xPath %s to filename: %v", xPath, err))

		return
	}

	v.lock(filename)
	defer v.unlock(filename)

	path := filepath.Join(v.Path, filename)
	emptyData := []byte("{}")

	if err := os.WriteFile(path, emptyData, v.fileMode); err != nil {
		v.log(fmt.Sprintf("Clear: error writing empty file for xPath %s: %v", xPath, err))
	}
}

func (v *JSONDB) lock(filename string) {
	jsonDBLocksMutex.Lock()

	mtx, ok := jsonDBLocks[filename]
	if !ok {
		mtx = &sync.Mutex{}
		jsonDBLocks[filename] = mtx
	}
	jsonDBLocksMutex.Unlock()
	mtx.Lock()
}

func (v *JSONDB) unlock(filename string) {
	jsonDBLocksMutex.Lock()
	if mtx, ok := jsonDBLocks[filename]; ok {
		mtx.Unlock()
	}
	jsonDBLocksMutex.Unlock()
}

func (v *JSONDB) xPathToFilename(xPath string) (string, error) {
	if pathComponents := strings.Split(xPath, v.xPathDelimeter); len(pathComponents) > 0 {
		return strings.ToLower(strings.Join(pathComponents, v.filenameDelimiter) + v.filenameExtension), nil
	}

	return "", errors.New("xPath has no components")
}

func (v *JSONDB) readJSONFileAsMap(filename string) (map[string]any, error) {
	var err error = nil

	jsonData := map[string]any{}
	path := filepath.Join(v.Path, filename)

	if fileData, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(fileData, &jsonData); err != nil {
			v.log(fmt.Sprintf("readJSONFileAsMap(%s) fileData: %s error", filename, fileData), err)
		}
	}

	return jsonData, err
}

func (v *JSONDB) writeMapAsJSONFile(filename string, o map[string]any) error {
	var err error = nil

	path := filepath.Join(v.Path, filename)
	if fileData, err := json.MarshalIndent(o, "", "  "); err == nil {
		if err = os.WriteFile(path, fileData, v.fileMode); err != nil {
			v.log(fmt.Sprintf("writeMapAsJSONFile path: %s, fileMode: %s, fileData: %s error", path, v.fileMode, fileData), err)
		}
	}

	return err
}

func (v *JSONDB) log(s string, params ...any) {
	if len(params) > 0 {
		log.TLogln(fmt.Sprintf("JSONDB: %s: %s", s, fmt.Sprint(params...)))
	} else {
		log.TLogln("JSONDB: " + s)
	}
}
