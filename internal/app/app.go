// Package app owns the gbay process lifecycle: directories, logging and the Bubble Tea program.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
	"github.com/JoaoVictorVM/gopherbay/internal/logging"
	"github.com/JoaoVictorVM/gopherbay/internal/version"
)

// Options configures a gbay run. Zero values select the real terminal and user directories.
type Options struct {
	// ConfigPath is the raw --config value; empty selects config.yaml in ConfigDir.
	ConfigPath string
	Debug      bool
	// ConfigDir holds the log and the default config file; empty selects DefaultConfigDir.
	ConfigDir string
	Input     io.Reader
	Output    io.Writer
	Stderr    io.Writer
	Start     time.Time
}

// Run opens the log, runs the interface until the user quits or ctx is
// canceled, and returns an error only when the interface fails.
func Run(ctx context.Context, opts Options) error {
	if opts.Start.IsZero() {
		opts.Start = time.Now()
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}

	configDir := opts.ConfigDir
	if configDir == "" {
		dir, err := DefaultConfigDir()
		if err != nil {
			return err
		}
		configDir = dir
	}
	configPath, err := ResolveConfigPath(opts.ConfigPath, configDir)
	if err != nil {
		return err
	}

	logger, closeLog := openLogger(configDir, opts)
	defer closeLog()

	// Stray slog or log calls from any package must never reach the terminal.
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)

	logger.Info("starting gbay",
		"version", version.Version,
		"commit", version.Commit,
		"date", version.Date,
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
		"go_version", runtime.Version(),
		"config_path", configPath,
		"debug", opts.Debug,
	)

	// Loading before the program starts lets the first frame show the config.
	// A failed load is not fatal: the interface opens in read-only mode with the error.
	store := config.NewStore(configPath, logger)
	_ = store.Load()

	err = runProgram(ctx, logger, opts, store)

	attrs := []any{"duration", time.Since(opts.Start)}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	logger.Info("gbay exited", attrs...)
	return err
}

// The log is not essential to use the launcher, so failing to open it only
// prints a warning that stays visible after the alternate screen closes.
func openLogger(dir string, opts Options) (logger *slog.Logger, closeLog func()) {
	level := slog.LevelInfo
	if opts.Debug {
		level = slog.LevelDebug
	}

	f, err := logging.Open(dir, level)
	if err != nil {
		_, _ = fmt.Fprintf(opts.Stderr, "gbay: aviso: não foi possível abrir o log em %s: %v\n", dir, err)
		return slog.New(slog.DiscardHandler), func() {}
	}
	if f.RotateErr != nil {
		f.Logger.Warn("log rotation failed", "error", f.RotateErr)
	}
	return f.Logger, func() { _ = f.Close() }
}

func runProgram(ctx context.Context, logger *slog.Logger, opts Options, store configStore) error {
	// SIGINT and SIGTERM arrive through ctx, so Bubble Tea must not turn them into errors.
	progOpts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithoutSignalHandler()}
	// tea.WithInput(nil) disables input entirely, so only override real readers.
	if opts.Input != nil {
		progOpts = append(progOpts, tea.WithInput(opts.Input))
	}
	if opts.Output != nil {
		progOpts = append(progOpts, tea.WithOutput(opts.Output))
	}

	_, err := tea.NewProgram(newModel(logger, opts.Start, store), progOpts...).Run()
	if err != nil && errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("executar interface: %w", err)
	}
	return nil
}
