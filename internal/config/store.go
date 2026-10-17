package config

import (
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// stamp identifies the file content seen at load, so a save can tell whether
// someone edited the file in the meantime.
type stamp struct {
	exists  bool
	modTime time.Time
	size    int64
	sum     [sha256.Size]byte
}

// Store owns the config file: it loads it, reports why it is invalid and
// saves the tool list back to the same path. It is safe for concurrent use.
type Store struct {
	path   string
	logger *slog.Logger
	parser configParser

	mu      sync.Mutex
	cfg     Config
	loadErr *Error
	stamp   stamp
	loads   int
}

// NewStore returns a store for the config file at path. It does no I/O.
func NewStore(path string, logger *slog.Logger) *Store {
	return &Store{path: path, logger: logger, parser: newParser(logger), cfg: Empty()}
}

// Path returns the config file path.
func (s *Store) Path() string {
	return s.path
}

// Config returns a copy of the loaded config, or Empty while read-only.
func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Clone()
}

// LoadError returns why the last load failed, or nil.
func (s *Store) LoadError() *Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadErr
}

// ReadOnly reports whether the last load failed. Saves are refused until a
// later load succeeds, so the invalid file is never overwritten.
func (s *Store) ReadOnly() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadErr != nil
}

// Load reads and parses the file. A missing file is an empty valid config and
// nothing is created. On failure the tool list is cleared, the store enters
// read-only mode and the returned error is an *Error.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reload := s.loads > 0
	s.loads++

	data, st, loadErr := s.read()
	var cfg Config
	if loadErr == nil {
		if !st.exists {
			cfg = Empty()
		} else {
			var err error
			cfg, err = s.parser.parse(data, s.path)
			if err != nil {
				loadErr = asLoadError(err, s.path)
			}
		}
	}

	if loadErr != nil {
		s.cfg, s.loadErr, s.stamp = Empty(), loadErr, stamp{}
		s.logger.Warn("config load failed",
			"path", s.path,
			"kind", string(loadErr.Kind),
			"line", loadErr.Line,
			"rule", string(loadErr.Rule),
			"violations", loadErr.Violations,
			"error", loadErr.Message,
		)
		return loadErr
	}

	s.cfg, s.loadErr, s.stamp = cfg, nil, st
	s.logger.Info("config loaded", "path", s.path, "tools", len(cfg.Tools), "missing", !st.exists, "reload", reload)
	return nil
}

// Save validates tools and atomically replaces the file with them, keeping the
// loaded release key. On any error the file and the in-memory config stay as
// they were: ErrReadOnly, *ValidationError, ErrChangedOnDisk or *SaveError.
func (s *Store) Save(tools []Tool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	if s.loadErr != nil {
		s.logger.Warn("config save aborted", "path", s.path, "reason", "read_only")
		return ErrReadOnly
	}
	tools = normalizeTools(tools)
	if violations := ValidateTools(tools); len(violations) > 0 {
		s.logger.Warn("config save aborted", "path", s.path, "reason", "invalid")
		return &ValidationError{Violations: violations}
	}
	changed, err := s.changedOnDisk()
	if err != nil {
		return s.saveFailed(&SaveError{Op: "verificar arquivo", Err: err})
	}
	if changed {
		s.logger.Warn("config save aborted", "path", s.path, "reason", "changed_on_disk")
		return ErrChangedOnDisk
	}

	cfg := Config{Version: CurrentVersion, Keys: s.cfg.Keys, Tools: tools}
	data, err := Marshal(cfg)
	if err != nil {
		return s.saveFailed(&SaveError{Op: "serializar", Err: err})
	}
	if err := writeFileAtomic(s.path, data, s.logger); err != nil {
		return s.saveFailed(err)
	}

	st := stamp{exists: true, sum: sha256.Sum256(data), size: int64(len(data))}
	if fi, err := os.Stat(s.path); err == nil {
		st.modTime, st.size = fi.ModTime(), fi.Size()
	}
	s.cfg, s.stamp = cfg.Clone(), st
	s.logger.Info("config saved", "path", s.path, "tools", len(tools), "duration", time.Since(start))
	return nil
}

// changedOnDisk compares the file with the load stamp. Modification time and
// size are cheap but coarse, so a match is confirmed with the content hash to
// catch a same size edit within the filesystem timestamp granularity.
func (s *Store) changedOnDisk() (bool, error) {
	fi, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s.stamp.exists, nil
	}
	if err != nil {
		return false, err
	}
	if !s.stamp.exists || fi.Size() != s.stamp.size || !fi.ModTime().Equal(s.stamp.modTime) {
		return true, nil
	}
	data, err := os.ReadFile(filepath.Clean(s.path))
	if err != nil {
		return false, err
	}
	return sha256.Sum256(data) != s.stamp.sum, nil
}

func (s *Store) saveFailed(err error) error {
	s.logger.Error("config save failed", "path", s.path, "error", err)
	return err
}

func (s *Store) read() ([]byte, stamp, *Error) {
	readErr := func(rule Rule, msg string, err error) *Error {
		return &Error{Kind: KindRead, Path: s.path, Rule: rule, Message: msg, Violations: 1, Err: err}
	}

	f, err := os.Open(filepath.Clean(s.path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, stamp{}, nil
	}
	if err != nil {
		return nil, stamp{}, readErr(RuleUnreadable, err.Error(), err)
	}
	defer f.Close()

	// The stamp comes from the open handle, before reading, so a write that
	// lands while reading makes the next save abort instead of being missed.
	fi, err := f.Stat()
	if err != nil {
		return nil, stamp{}, readErr(RuleUnreadable, err.Error(), err)
	}
	if fi.IsDir() {
		return nil, stamp{}, readErr(RuleUnreadable, "o caminho é uma pasta", nil)
	}
	if fi.Size() > MaxFileSize {
		return nil, stamp{}, readErr(RuleFileTooLarge, "o arquivo tem mais de 1 MiB", nil)
	}

	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, stamp{}, readErr(RuleUnreadable, err.Error(), err)
	}
	return data, stamp{exists: true, modTime: fi.ModTime(), size: fi.Size(), sum: sha256.Sum256(data)}, nil
}

func asLoadError(err error, path string) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Kind: KindSyntax, Path: path, Rule: RuleSyntax, Message: err.Error(), Violations: 1, Err: err}
}
