//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

type NativeInfo struct {
	UptimeSeconds uint64
	Shell         string
	Disks         []string
	Terminal      string
}

type MouseState struct {
	X, Y  int
	Left  bool
	Moved bool
}

type InputEvent struct {
	Key   string
	Mouse *MouseState
}

var (
	k32                            = syscall.NewLazyDLL("kernel32.dll")
	pGetStdHandle                  = k32.NewProc("GetStdHandle")
	pGetConsoleMode                = k32.NewProc("GetConsoleMode")
	pSetConsoleMode                = k32.NewProc("SetConsoleMode")
	pSetConsoleOutputCP            = k32.NewProc("SetConsoleOutputCP")
	pSetConsoleCP                  = k32.NewProc("SetConsoleCP")
	pGetConsoleScreenBufferInfo    = k32.NewProc("GetConsoleScreenBufferInfo")
	pGetTickCount64                = k32.NewProc("GetTickCount64")
	pGetLogicalDrives              = k32.NewProc("GetLogicalDrives")
	pGetDriveTypeW                 = k32.NewProc("GetDriveTypeW")
	pGetVolumeInformationW         = k32.NewProc("GetVolumeInformationW")
	pCreateToolhelp32Snapshot      = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateToolhelp32Snapshot")
	pProcess32FirstW               = syscall.NewLazyDLL("kernel32.dll").NewProc("Process32FirstW")
	pProcess32NextW                = syscall.NewLazyDLL("kernel32.dll").NewProc("Process32NextW")
	pCloseHandle                   = k32.NewProc("CloseHandle")
	pGetNumberOfConsoleInputEvents = k32.NewProc("GetNumberOfConsoleInputEvents")
	pReadConsoleInputW             = k32.NewProc("ReadConsoleInputW")
)

const (
	stdInputHandle           = ^uintptr(9)  // -10
	stdOutputHandle          = ^uintptr(10) // -11
	enableLineInput          = 0x0002
	enableEchoInput          = 0x0004
	enableWindowInput        = 0x0008
	enableMouseInput         = 0x0010
	enableQuickEditMode      = 0x0040
	enableExtendedFlags      = 0x0080
	enableVTProcessing       = 0x0004
	disableNewlineAutoReturn = 0x0008
	th32csSnapProcess        = 0x00000002
	mouseMoved               = 0x0001
	fromLeftButton           = 0x0001
	driveRemovable           = 2
	driveFixed               = 3
	driveRemote              = 4
)

type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}
type processEntry32W struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}
type inputRecord struct {
	EventType uint16
	Padding   uint16
	Event     [16]byte
}

var cachedShellInfo string

func getHandle(which uintptr) uintptr { h, _, _ := pGetStdHandle.Call(which); return h }

func startTerminal(mouse bool) (func(), error) {
	inHandle, outHandle := getHandle(stdInputHandle), getHandle(stdOutputHandle)
	var oldIn, oldOut uint32
	ok1, _, _ := pGetConsoleMode.Call(inHandle, uintptr(unsafe.Pointer(&oldIn)))
	ok2, _, _ := pGetConsoleMode.Call(outHandle, uintptr(unsafe.Pointer(&oldOut)))
	if ok1 == 0 || ok2 == 0 {
		return func() {}, fmt.Errorf("Fetch requires an interactive Windows console (Windows Terminal is recommended)")
	}
	newOut := oldOut | enableVTProcessing | disableNewlineAutoReturn
	pSetConsoleOutputCP.Call(65001)
	pSetConsoleCP.Call(65001)
	if ok, _, _ := pSetConsoleMode.Call(outHandle, uintptr(newOut)); ok == 0 {
		return func() {}, fmt.Errorf("could not enable ANSI terminal output")
	}
	newIn := oldIn | enableExtendedFlags | enableWindowInput
	newIn &^= enableLineInput | enableEchoInput | enableQuickEditMode
	if mouse {
		newIn |= enableMouseInput
	}
	if ok, _, _ := pSetConsoleMode.Call(inHandle, uintptr(newIn)); ok == 0 {
		pSetConsoleMode.Call(outHandle, uintptr(oldOut))
		return func() {}, fmt.Errorf("could not enable terminal input events")
	}
	cleanup := func() {
		pSetConsoleMode.Call(inHandle, uintptr(oldIn))
		pSetConsoleMode.Call(outHandle, uintptr(oldOut))
	}
	return cleanup, nil
}

func terminalSize() (int, int) {
	h := getHandle(stdOutputHandle)
	var info consoleScreenBufferInfo
	ok, _, _ := pGetConsoleScreenBufferInfo.Call(h, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 100, 35
	}
	w := int(info.Window.Right - info.Window.Left + 1)
	hgt := int(info.Window.Bottom - info.Window.Top + 1)
	if w < 1 {
		w = int(info.Size.X)
	}
	if hgt < 1 {
		hgt = int(info.Size.Y)
	}
	return w, hgt
}

func readShell() string {
	if cachedShellInfo != "" {
		return cachedShellInfo
	}
	const invalidHandle = ^uintptr(0)
	snapshot, _, _ := pCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snapshot == invalidHandle {
		cachedShellInfo = fallbackShell()
		return cachedShellInfo
	}
	defer pCloseHandle.Call(snapshot)
	processes := map[uint32]processEntry32W{}
	var entry processEntry32W
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, _ := pProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		processes[entry.ProcessID] = entry
		entry.Size = uint32(unsafe.Sizeof(entry))
		ok, _, _ = pProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	}
	pid := uint32(syscall.Getpid())
	foundCmd := false
	for i := 0; i < 12; i++ {
		p, exists := processes[pid]
		if !exists {
			break
		}
		name := strings.ToLower(syscall.UTF16ToString(p.ExeFile[:]))
		switch name {
		case "pwsh.exe":
			cachedShellInfo = shellVersion("pwsh.exe", "--version", "PowerShell 7")
			return cachedShellInfo
		case "powershell.exe":
			cachedShellInfo = shellVersion("powershell.exe", "-NoProfile", "Windows PowerShell")
			return cachedShellInfo
		case "cmd.exe":
			foundCmd = true
		}
		if p.ParentProcessID == 0 || p.ParentProcessID == pid {
			break
		}
		pid = p.ParentProcessID
	}
	if foundCmd {
		cachedShellInfo = "Command Prompt"
	} else {
		cachedShellInfo = fallbackShell()
	}
	return cachedShellInfo
}

func shellVersion(executable, firstArg, fallback string) string {
	var cmd *exec.Cmd
	if strings.EqualFold(executable, "pwsh.exe") {
		cmd = exec.Command(executable, firstArg)
	} else {
		cmd = exec.Command(executable, firstArg, "-NonInteractive", "-Command", "$PSVersionTable.PSVersion.ToString()")
	}
	out, err := cmd.Output()
	if err != nil {
		return fallback
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return fallback
	}
	if strings.EqualFold(executable, "pwsh.exe") {
		return version
	}
	return "Windows PowerShell " + version
}

func readTerminal() string {
	if os.Getenv("WT_SESSION") != "" {
		return "Windows Terminal"
	}
	if os.Getenv("WEZTERM_PANE") != "" {
		return "WezTerm"
	}
	if os.Getenv("ALACRITTY_SOCKET") != "" {
		return "Alacritty"
	}
	if os.Getenv("TERM_PROGRAM") != "" {
		return os.Getenv("TERM_PROGRAM")
	}
	return "Unknown"
}

func fallbackShell() string {
	psPath, _ := syscall.Getenv("PSModulePath")
	if strings.Contains(strings.ToLower(psPath), "powershell") {
		return "PowerShell"
	}
	return "Unknown"
}

func readDisks() []string {
	mask, _, _ := pGetLogicalDrives.Call()
	var items []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := rune('A' + i)
		root := string(letter) + ":\\"
		root16, _ := syscall.UTF16PtrFromString(root)
		driveType, _, _ := pGetDriveTypeW.Call(uintptr(unsafe.Pointer(root16)))
		if driveType != driveFixed && driveType != driveRemovable && driveType != driveRemote {
			continue
		}
		volume := make([]uint16, 261)
		fs := make([]uint16, 64)
		ok, _, _ := pGetVolumeInformationW.Call(uintptr(unsafe.Pointer(root16)), uintptr(unsafe.Pointer(&volume[0])), uintptr(len(volume)), 0, 0, 0, uintptr(unsafe.Pointer(&fs[0])), uintptr(len(fs)))
		label := ""
		if ok != 0 {
			label = strings.TrimSpace(syscall.UTF16ToString(volume))
		}
		items = append(items, formatVolume(root, label, ""))
	}
	return items
}

func readNativeInfo() NativeInfo {
	ms, _, _ := pGetTickCount64.Call()
	return NativeInfo{UptimeSeconds: uint64(ms / 1000), Shell: readShell(), Terminal: readTerminal(), Disks: readDisks()}
}

func pollEvents() []InputEvent {
	h := getHandle(stdInputHandle)
	var count uint32
	ok, _, _ := pGetNumberOfConsoleInputEvents.Call(h, uintptr(unsafe.Pointer(&count)))
	if ok == 0 || count == 0 {
		return nil
	}
	if count > 128 {
		count = 128
	}
	records := make([]inputRecord, count)
	var read uint32
	ok, _, _ = pReadConsoleInputW.Call(h, uintptr(unsafe.Pointer(&records[0])), uintptr(count), uintptr(unsafe.Pointer(&read)))
	if ok == 0 || read == 0 {
		return nil
	}
	out := make([]InputEvent, 0, read)
	for _, rec := range records[:read] {
		switch rec.EventType {
		case 0x0001: // KEY_EVENT
			keyDown := uint32(rec.Event[0]) | uint32(rec.Event[1])<<8 | uint32(rec.Event[2])<<16 | uint32(rec.Event[3])<<24
			if keyDown == 0 {
				continue
			}
			ch := uint16(rec.Event[10]) | uint16(rec.Event[11])<<8
			vk := uint16(rec.Event[6]) | uint16(rec.Event[7])<<8
			key := ""
			// Check virtual keys before Unicode characters: Enter commonly carries '\r'.
			if vk == 0x1B {
				key = "escape"
			} else if vk == 0x0D {
				key = "enter"
			} else if ch != 0 {
				key = string(rune(ch))
			}
			if key != "" {
				out = append(out, InputEvent{Key: key})
			}
		case 0x0002: // MOUSE_EVENT
			x := int(int16(uint16(rec.Event[0]) | uint16(rec.Event[1])<<8))
			y := int(int16(uint16(rec.Event[2]) | uint16(rec.Event[3])<<8))
			var screen consoleScreenBufferInfo
			if ok, _, _ := pGetConsoleScreenBufferInfo.Call(getHandle(stdOutputHandle), uintptr(unsafe.Pointer(&screen))); ok != 0 {
				x -= int(screen.Window.Left)
				y -= int(screen.Window.Top)
			}
			buttons := uint32(rec.Event[4]) | uint32(rec.Event[5])<<8 | uint32(rec.Event[6])<<16 | uint32(rec.Event[7])<<24
			flags := uint16(rec.Event[12]) | uint16(rec.Event[13])<<8
			out = append(out, InputEvent{Mouse: &MouseState{X: x, Y: y, Left: buttons&fromLeftButton != 0, Moved: flags&mouseMoved != 0}})
		}
	}
	return out
}

func cpuNameFallback() string {
	out, err := exec.Command("reg.exe", "query", `HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "/v", "ProcessorNameString").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "REG_SZ") {
			parts := strings.SplitN(line, "REG_SZ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}
