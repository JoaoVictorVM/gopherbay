//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// Console programs only get their escape sequences interpreted, ConPTY
// included, after enabling virtual terminal processing on the output handle.
func enableVirtualTerminal(f *os.File) (restore func(), err error) {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return func() {}, err
	}
	if err := windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return func() {}, err
	}
	return func() { _ = windows.SetConsoleMode(h, mode) }, nil
}
