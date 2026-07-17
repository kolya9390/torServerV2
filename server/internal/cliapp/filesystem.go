package cliapp

import (
	"io/fs"
	"os"
)

// FileSystem is the local filesystem surface needed by CLI commands.
type FileSystem interface {
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
	MkdirAll(string, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
	UserConfigDir() (string, error)
}

type osFileSystem struct{}

func (osFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (osFileSystem) WriteFile(name string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(name, data, mode)
}

func (osFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (osFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (osFileSystem) UserConfigDir() (string, error) {
	return os.UserConfigDir()
}
