/*
Copyright The Velero Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filesystem

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Interface defines methods for interacting with an
// underlying file system.
type Interface interface {
	TempDir(dir, prefix string) (string, error)
	MkdirAll(path string, perm os.FileMode) error
	Create(name string) (io.WriteCloser, error)
	OpenFile(name string, flag int, perm os.FileMode) (io.WriteCloser, error)
	RemoveAll(path string) error
	ReadDir(dirname string) ([]os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	DirExists(path string) (bool, error)
	TempFile(dir, prefix string) (NameWriteCloser, error)
	Stat(path string) (os.FileInfo, error)
	Glob(path string) ([]string, error)
}

type NameWriteCloser interface {
	io.WriteCloser

	Name() string
}

func NewFileSystem() Interface {
	return &osFileSystem{}
}

type osFileSystem struct{}

func (fs *osFileSystem) Glob(path string) ([]string, error) {
	return filepath.Glob(path)
}

func (fs *osFileSystem) TempDir(dir, prefix string) (string, error) {
	return ioutil.TempDir(dir, prefix)
}

func (fs *osFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fs *osFileSystem) Create(name string) (io.WriteCloser, error) {
	return os.Create(name)
}

func (fs *osFileSystem) OpenFile(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(name, flag, perm)
}

func (fs *osFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *osFileSystem) ReadDir(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
}

func (fs *osFileSystem) ReadFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filename)
}

func (fs *osFileSystem) DirExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (fs *osFileSystem) TempFile(dir, prefix string) (NameWriteCloser, error) {
	return ioutil.TempFile(dir, prefix)
}

func (fs *osFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
