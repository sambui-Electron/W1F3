//go:build !windows

package main

import (
	"time"
)

type NativeInfo struct {
	UptimeSeconds   uint64
	Shell, Terminal string
	Disks           []string
}
type MouseState struct {
	X, Y        int
	Left, Moved bool
}
type InputEvent struct {
	Key   string
	Mouse *MouseState
}

func startTerminal(mouse bool) (func(), error) { return func() {}, nil }
func terminalSize() (int, int)                 { return 110, 36 }
func pollEvents() []InputEvent                 { return nil }
func readNativeInfo() NativeInfo {
	return NativeInfo{UptimeSeconds: uint64(time.Since(time.Now().Add(-48 * time.Hour)).Seconds()), Shell: "PowerShell", Terminal: "Windows Terminal", Disks: []string{"C:\\Windows"}}
}
func cpuNameFallback() string { return "" }
