package config

import (
	"fmt"
	"log/slog"

	"github.com/goccy/go-yaml/ast"
)

// migrateFunc upgrades a document from one version to the next by editing
// the AST in place, so untouched nodes keep their line numbers.
type migrateFunc func(body ast.Node) error

// migrations maps a source version to the step that upgrades it to the next
// one. Version 1 is the first format, so there is nothing to migrate yet.
var migrations = map[int]migrateFunc{}

func newParser(logger *slog.Logger) configParser {
	return configParser{current: CurrentVersion, migrations: migrations, logger: logger}
}

// migrate applies every step from version to p.current. The whole chain is
// checked before the first step runs, so a gap never leaves a half migrated tree.
func (p configParser) migrate(body ast.Node, version int, line int, path string) error {
	for v := version; v < p.current; v++ {
		if p.migrations[v] == nil {
			return validationError(path, line, RuleVersionTooOld, fmt.Sprintf("Versão %d do config é inválida (use version: %d)", version, p.current))
		}
	}
	for v := version; v < p.current; v++ {
		if err := p.migrations[v](body); err != nil {
			e := validationError(path, line, RuleVersionTooOld, fmt.Sprintf("não foi possível migrar a versão %d do config: %v", v, err))
			e.Err = err
			return e
		}
	}
	p.logger.Info("config migrated", "from", version, "to", p.current)
	return nil
}
