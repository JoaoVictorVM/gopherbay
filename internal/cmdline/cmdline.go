// Package cmdline splits a tool command line into the program and its arguments.
package cmdline

import (
	"errors"
	"strings"
)

// ErrUnclosedQuote reports a single or double quote that is never closed.
var ErrUnclosedQuote = errors.New("aspas sem fechamento")

// Split breaks line into arguments. Spaces and tabs outside quotes separate
// arguments, single and double quotes group text and are removed, and a quote
// of the other kind is literal inside them.
//
// Backslash is always literal, unlike shell splitters, so unquoted Windows
// paths such as C:\tools\clock.exe survive intact. There is no environment
// expansion, globbing or shell operator handling: the command runs directly,
// never through a shell.
func Split(line string) ([]string, error) {
	args := []string{}
	var cur strings.Builder
	// inArg tells an empty quoted argument ("") apart from no argument at all.
	inArg := false
	var quote rune

	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inArg = true
		case r == ' ' || r == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}

	if quote != 0 {
		return nil, ErrUnclosedQuote
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}
