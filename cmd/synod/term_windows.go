//go:build windows

package main

import (
	"io"
	"os"
	"syscall"
)

const enableVirtualTerminalProcessing = 0x0004

var setConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")

// enableVTReal turns on ENABLE_VIRTUAL_TERMINAL_PROCESSING for the console
// behind w. It reports false when w is not a console or the call fails; the
// output then stays rich but without color.
func enableVTReal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return false
	}
	h := syscall.Handle(f.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	r, _, _ := setConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}
