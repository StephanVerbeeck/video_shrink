package main

import (
	"os"
	"syscall"
	"unsafe"
)

func init() {
	// Activeer Virtual Terminal Processing zodat ANSI-kleurcodes werken bij auto-start
	const ENABLE_VIRTUAL_TERMINAL_PROCESSING uint32 = 0x0004
	handle := syscall.Handle(os.Stdout.Fd())
	var mode uint32

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	// Vraag de huidige console-modus op en voeg de vlag toe
	ret, _, _ := getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if ret != 0 {
		mode |= ENABLE_VIRTUAL_TERMINAL_PROCESSING
		setConsoleMode.Call(uintptr(handle), uintptr(mode))
	}
}
