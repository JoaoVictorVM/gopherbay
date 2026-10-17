//go:build !windows

package main

import "os"

func enableVirtualTerminal(*os.File) (restore func(), err error) {
	return func() {}, nil
}
