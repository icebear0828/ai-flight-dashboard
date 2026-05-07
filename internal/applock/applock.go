package applock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrAlreadyLocked = errors.New("application data directory is already locked")

type Lock struct {
	file    *os.File
	path    string
	release func() error
}

func TryAcquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	release, err := tryLockFile(file)
	if err != nil {
		file.Close()
		return nil, err
	}

	if err := file.Truncate(0); err != nil {
		release()
		file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, 0); err != nil {
		release()
		file.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		release()
		file.Close()
		return nil, err
	}

	return &Lock{file: file, path: path, release: release}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.release()
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

func (l *Lock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}
