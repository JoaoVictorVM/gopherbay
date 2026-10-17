package config

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	staleTempAge      = time.Minute
	renameRetryWindow = 500 * time.Millisecond
	renameRetryDelay  = 25 * time.Millisecond
	// errSharingViolation is ERROR_SHARING_VIOLATION on Windows.
	errSharingViolation syscall.Errno = 32
)

// beforeRename runs after the temporary file is flushed and closed, right
// before it replaces the target. Only tests set it, to simulate a failure or
// a crash at the one moment where both files exist.
var beforeRename func(tmp string) error

// writeFileAtomic replaces target with data so that a crash at any point
// leaves either the old or the new complete file, never a partial one.
func writeFileAtomic(target string, data []byte, logger *slog.Logger) error {
	// Dotfile managers keep the config as a symlink; renaming over the link
	// would replace it with a regular file, so the real file is replaced instead.
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return &SaveError{Op: "resolver link", Err: err}
		}
		resolved = target
	}
	dir, base := filepath.Dir(resolved), filepath.Base(resolved)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return &SaveError{Op: "criar pasta", Err: err}
	}

	mode := fs.FileMode(0o600)
	if fi, err := os.Stat(resolved); err == nil {
		mode = fi.Mode().Perm()
	}

	// The temporary file sits next to the target because a rename is only
	// atomic within one filesystem.
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return &SaveError{Op: "criar arquivo temporário", Err: err}
	}
	tmpName := tmp.Name()
	fail := func(op string, err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return &SaveError{Op: op, Err: err}
	}

	if _, err := tmp.Write(data); err != nil {
		return fail("gravar", err)
	}
	if err := tmp.Sync(); err != nil {
		return fail("gravar", err)
	}
	if err := tmp.Close(); err != nil {
		return fail("gravar", err)
	}
	// Windows only maps the read-only bit, which a new temp file never has.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpName, mode); err != nil {
			return fail("ajustar permissões", err)
		}
	}
	if beforeRename != nil {
		if err := beforeRename(tmpName); err != nil {
			return fail("renomear", err)
		}
	}
	if err := renameWithRetry(tmpName, resolved); err != nil {
		return fail("renomear", err)
	}

	syncDir(dir)
	removeStaleTemps(dir, base, logger)
	return nil
}

// Antivirus scanners and indexers briefly lock freshly written files on
// Windows, which makes the replace fail with access denied or a sharing violation.
func renameWithRetry(from, to string) error {
	deadline := time.Now().Add(renameRetryWindow)
	for {
		err := os.Rename(from, to)
		if err == nil || runtime.GOOS != "windows" || !transientRenameError(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(renameRetryDelay)
	}
}

func transientRenameError(err error) bool {
	var errno syscall.Errno
	return errors.Is(err, fs.ErrPermission) || (errors.As(err, &errno) && errno == errSharingViolation)
}

// Syncing the folder makes the rename itself durable on Linux and macOS.
// Windows cannot open a folder this way, and a failure only weakens durability,
// never atomicity, so errors are ignored.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(filepath.Clean(dir))
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// A crash between creating and renaming the temporary file leaves it behind.
// Recent ones may belong to another gbay saving right now, so only old ones go.
func removeStaleTemps(dir, base string, logger *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := "." + base + "."
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < staleTempAge {
			continue
		}
		file := filepath.Join(dir, name)
		if os.Remove(file) == nil {
			logger.Debug("stale temp file removed", "file", file)
		}
	}
}
