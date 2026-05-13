package ffprobe

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/vansante/go-ffprobe.v2"
)

var binFile = "ffprobe"

const probeTimeout = 30 * time.Second

func init() {
	path, err := exec.LookPath("ffprobe")
	if err == nil {
		ffprobe.SetFFProbeBinPath(path)
		binFile = path
	} else {
		// working dir
		if _, err := os.Stat("ffprobe"); os.IsNotExist(err) {
			ffprobe.SetFFProbeBinPath(filepath.Dir(os.Args[0]) + "/ffprobe")
			binFile = filepath.Dir(os.Args[0]) + "/ffprobe"
		}
	}
}

func Exists() bool {
	_, err := os.Stat(binFile)

	return !os.IsNotExist(err)
}

func ProbeURL(link string) (*ffprobe.ProbeData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	data, err := ffprobe.ProbeURL(ctx, link)

	return data, err
}

func ProbeReader(reader io.Reader) (*ffprobe.ProbeData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	data, err := ffprobe.ProbeReader(ctx, reader)

	return data, err
}
