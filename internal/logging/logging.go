// Package logging writes the gbay log file, keeping the terminal reserved for the interface.
package logging

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	FileName    = "gbay.log"
	OldFileName = "gbay.log.old"
)

// File is an open log file and the logger that writes to it.
type File struct {
	Logger *slog.Logger
	// RotateErr is not fatal: the previous log is kept and the new entries are appended to it.
	RotateErr error
	Path      string

	f *os.File
}

// Open creates dir when missing, renames the previous log to OldFileName and
// opens a fresh FileName that records entries at level and above.
func Open(dir string, level slog.Level) (*File, error) {
	// Log entries contain commands and folders, so only the user can read them.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("criar pasta do log: %w", err)
	}

	path := filepath.Join(dir, FileName)
	rotateErr := rotate(path, filepath.Join(dir, OldFileName))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // path is built from the config dir, not from user input
	if err != nil {
		return nil, fmt.Errorf("abrir arquivo de log: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))
	return &File{Logger: logger, RotateErr: rotateErr, Path: path, f: f}, nil
}

// Close flushes and closes the log file.
func (l *File) Close() error {
	return l.f.Close()
}

// A second gbay running at the same time keeps the file open on Windows, so the
// rename fails there and the new process appends instead.
func rotate(path, oldPath string) error {
	err := os.Rename(path, oldPath)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("renomear log anterior: %w", err)
}
