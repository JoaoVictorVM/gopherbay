// Command gbay is the gopherbay launcher.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JoaoVictorVM/gopherbay/internal/app"
	"github.com/JoaoVictorVM/gopherbay/internal/version"
)

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

const usage = `Uso: gbay [opções]

Opções:
  --config <arquivo>  usa este arquivo de configuração em vez do padrão
  --debug             grava mensagens de depuração no log
  --version           mostra a versão e sai
`

// Captured at package initialization so the startup time includes everything before main.
var processStart = time.Now()

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// A nil input lets Bubble Tea fall back to the TTY when stdin is redirected.
	code := run(ctx, os.Args[1:], nil, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gbay", flag.ContinueOnError)
	// The flag package prints in English; errors and usage are printed below in pt-BR.
	fs.SetOutput(io.Discard)
	showVersion := fs.Bool("version", false, "")
	configPath := fs.String("config", "", "")
	debug := fs.Bool("debug", false, "")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(stdout, usage)
			return exitOK
		}
		_, _ = fmt.Fprintf(stderr, "gbay: argumentos inválidos: %v\n%s", err, usage)
		return exitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "gbay: argumento inesperado: %s\n%s", fs.Arg(0), usage)
		return exitUsage
	}

	if *showVersion {
		_, _ = fmt.Fprintln(stdout, version.String())
		return exitOK
	}

	err := app.Run(ctx, app.Options{
		ConfigPath: *configPath,
		Debug:      *debug,
		Input:      stdin,
		Output:     stdout,
		Stderr:     stderr,
		Start:      processStart,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "gbay: não foi possível executar a interface: %v\n", err)
		return exitFailure
	}
	return exitOK
}
