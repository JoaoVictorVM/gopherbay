package config

import (
	"errors"
	"fmt"
)

// ErrorKind classifies a load error.
type ErrorKind string

// Load error kinds.
const (
	KindSyntax     ErrorKind = "syntax"
	KindValidation ErrorKind = "validation"
	KindRead       ErrorKind = "read"
)

// Error is a config file that could not be loaded. The messages always say
// config.yaml, even with --config, because the pane shows Path right below.
type Error struct {
	Kind ErrorKind
	Path string
	// Line is 1 based; 0 means the position is unknown.
	Line    int
	Rule    Rule
	Message string
	// Violations counts every rule broken in the file, of which only the one on
	// the lowest line is reported.
	Violations int
	Err        error
}

func (e *Error) Error() string {
	if e.Kind == KindRead {
		return "Não foi possível ler o config.yaml: " + e.Message
	}
	if e.Line > 0 {
		return fmt.Sprintf("Erro no config.yaml (linha %d): %s", e.Line, e.Message)
	}
	return "Erro no config.yaml: " + e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

// ErrReadOnly is returned by Save while the loaded file is invalid, because
// writing would discard the content the user still has to fix.
var ErrReadOnly = errors.New("config inválido: corrija o arquivo e aperte r para recarregar")

// ErrChangedOnDisk is returned by Save when the file differs from what was loaded.
var ErrChangedOnDisk = errors.New("config.yaml mudou fora do gbay. Aperte r para recarregar (sua alteração não foi salva)")

// ValidationError is a tool list refused by Save.
type ValidationError struct {
	Violations []Violation
}

func (e *ValidationError) Error() string {
	if len(e.Violations) == 0 {
		return "lista de ferramentas inválida"
	}
	return e.Violations[0].Message
}

// SaveError is a failed write. The original file is intact.
type SaveError struct {
	Op  string
	Err error
}

func (e *SaveError) Error() string {
	return fmt.Sprintf("Não foi possível salvar o config.yaml: %s: %v", e.Op, e.Err)
}

func (e *SaveError) Unwrap() error {
	return e.Err
}
