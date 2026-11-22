package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigYAMLDefaultsAndOverrides(t *testing.T) {
	cfg, err := parseYAMLConfig([]byte(`# comment
logo: "Windows 11"
speed: 0.8
mouse: true
fields:
  - os
  - kernel
  - uptime
`), defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logo != "Windows 11" || math.Abs(cfg.Speed-0.8) > 0.0001 || !cfg.Mouse {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if strings.Join(cfg.Fields, ",") != "os,kernel,uptime" {
		t.Fatalf("fields not parsed: %#v", cfg.Fields)
	}
}

func TestUptimeUsesSeconds(t *testing.T) {
	got := fmtUptime(78*86400 + 12*3600 + 43*60 + 50)
	if got != "78d 12h 43m" {
		t.Fatalf("got %q", got)
	}
	if got := fmtUptime(59); got != "0m" {
		t.Fatalf("short uptime = %q", got)
	}
}

func TestCPUJSONUsesFastfetchCPUField(t *testing.T) {
	cpu := map[string]any{
		"cpu":   "AMD Ryzen 7 9700X 8-Core Processor",
		"cores": map[string]any{"physical": float64(8), "logical": float64(16)},
	}
	got := fmtCPU(cpu)
	want := "AMD Ryzen 7 9700X 8-Core Processor · 8 cores, 16 threads"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLogoANSIParserAndPointCloud(t *testing.T) {
	rows := parseLogo("\x1b[38;5;39m██\x1b[0m\n██\n")
	if len(rows) != 2 || len(rows[0]) != 2 || rows[0][0].Color == "" {
		t.Fatalf("logo parsing failed: %#v", rows)
	}
	points, w, h := makePoints(rows, 1)
	if w != 2 || h != 2 || len(points) == 0 {
		t.Fatalf("invalid point cloud: %d %d %d", len(points), w, h)
	}
	frame, _ := renderLogo(points, w, h, 20, 10, .2, .3, 1, true)
	if len(frame) != 10 || len(frame[0]) != 20 {
		t.Fatalf("invalid render size")
	}
}

func TestCompactSideBySideInfoLayout(t *testing.T) {
	cfg := defaultConfig()
	info := []InfoLine{{Label: "OS", Value: "Windows 11 Pro"}, {Label: "Kernel", Value: "Win32_NT"}, {Label: "Uptime", Value: "78d 12h 43m"}}
	logo := [][]Pixel{{{'#', cCyan, 0}, {'#', cCyan, 0}}, {{'#', cCyan, 0}, {'#', cCyan, 0}}}
	out, bounds := composeFrame(logo, info, "SAM@PC", 110, 35, cfg)
	if !strings.Contains(out, "Win32_NT") || !strings.Contains(out, "SAM@PC") {
		t.Fatalf("expected content not in frame")
	}
	if bounds.W < 30 || bounds.H < 20 {
		t.Fatalf("unexpected logo viewport: %#v", bounds)
	}
	if strings.HasSuffix(out, "\r\n") {
		t.Fatal("frame must not scroll the terminal on the last row")
	}
}

func TestDiskLabelsRemainReadable(t *testing.T) {
	if got := formatVolume(`C:\`, "Windows", "Local Disk"); got != `C:\Windows` {
		t.Fatalf("got %q", got)
	}
	if got := formatVolume(`D:\`, "", "Local Disk"); got != `D:\` {
		t.Fatalf("got %q", got)
	}
}

func TestFastfetchDecoderAcceptsArrayResults(t *testing.T) {
	input := []byte(`[{"type":"Display","result":[{"name":"Panel A"}]},{"type":"GPU","result":[{"name":"Adapter A"}]},{"type":"CPU","result":{"cpu":"CPU model"}}]`)
	modules, err := decodeFastfetchJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := modules["display"].([]any); !ok {
		t.Fatalf("display result should remain an array: %#v", modules["display"])
	}
	if got := fmtCPU(asMap(modules["cpu"])); got != "CPU model" {
		t.Fatalf("CPU model lost: %q", got)
	}
}

func TestMouseDragUpdatesAnglesAndLeavesMomentum(t *testing.T) {
	drag := DragState{}
	bounds := Rect{X: 2, Y: 3, W: 20, H: 12}
	ax, ay := 0.1, 0.2
	drag.Update(MouseState{X: 5, Y: 6, Left: true}, bounds, &ax, &ay)
	if !drag.Active || ax != 0.1 || ay != 0.2 {
		t.Fatalf("drag did not start correctly")
	}
	drag.Update(MouseState{X: 10, Y: 14, Left: true, Moved: true}, bounds, &ax, &ay)
	if math.Abs(ax-(0.1+8*0.018)) > 0.00001 || math.Abs(ay-(0.2+5*0.018)) > 0.00001 {
		t.Fatalf("drag did not update angles: %.4f %.4f", ax, ay)
	}
	drag.Update(MouseState{X: 10, Y: 14, Left: false}, bounds, &ax, &ay)
	if drag.Active || drag.VelocityX == 0 || drag.VelocityY == 0 {
		t.Fatalf("release should preserve fling velocity")
	}
}

func TestMissingConfigIsDownloadedOnFirstRun(t *testing.T) {
	oldDownloader := downloadDefaultConfig
	downloadDefaultConfig = func() ([]byte, error) {
		return []byte("logo: \"Windows 11\"\nmouse: true\nfields:\n  - os\n  - kernel\n"), nil
	}
	t.Cleanup(func() { downloadDefaultConfig = oldDownloader })
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	cfg, gotPath, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != path || cfg.Logo != "Windows 11" || !cfg.Mouse {
		t.Fatalf("unexpected downloaded config: %#v %q", cfg, gotPath)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file was not created: %v", err)
	}
}

func TestResponsiveLayoutKeepsLogoAndPanelTogether(t *testing.T) {
	layout := computeLayout(238, 67, true)
	if layout.Logo.X <= 0 || layout.InfoX <= layout.Logo.X+layout.Logo.W || layout.InfoX+layout.InfoW >= 238 {
		t.Fatalf("invalid centered composition: %#v", layout)
	}
	gap := layout.InfoX - (layout.Logo.X + layout.Logo.W)
	if gap > 7 {
		t.Fatalf("logo and panel gap is too large: %d", gap)
	}
}

func TestAdaptiveScaleIsIndependentOfUserScale(t *testing.T) {
	cfg := defaultConfig()
	small := adaptiveScale(80, 24, cfg)
	large := adaptiveScale(238, 67, cfg)
	if small >= large || large > cfg.AdaptiveMax || small < cfg.AdaptiveMin {
		t.Fatalf("unexpected adaptive scale range: small=%.2f large=%.2f", small, large)
	}
	cfg.AdaptiveScale = false
	if got := adaptiveScale(238, 67, cfg); got != 1 {
		t.Fatalf("adaptive scaling disabled, got %.2f", got)
	}
}

func TestAdaptiveScaleNeverShrinksBelowOne(t *testing.T) {
	cfg := defaultConfig()
	for _, dims := range [][2]int{{40, 12}, {80, 24}, {120, 36}, {238, 67}} {
		if got := adaptiveScale(dims[0], dims[1], cfg); got < 1.0 {
			t.Fatalf("adaptive multiplier must not shrink: %v => %.2f", dims, got)
		}
	}
}

func TestDiskInfoUsesMultilineList(t *testing.T) {
	got := buildInfo(nil, []string{"disk"}, NativeInfo{Disks: []string{`C:\Windows`, `D:\example`}})
	if len(got) != 1 || got[0].Label != "Disk" || len(got[0].Lines) != 2 {
		t.Fatalf("unexpected disk info: %#v", got)
	}
}

func TestVersionFlagIsDocumented(t *testing.T) {
	if !strings.Contains(version, "0.5.0") {
		t.Fatalf("unexpected W1F3 version: %s", version)
	}
}

func TestDiskListRendersOnSeparateBulletRows(t *testing.T) {
	cfg := defaultConfig()
	info := []InfoLine{{Label: "Disk", Lines: []string{`C:\Windows`, `D:\example`, `E:\example2`}}}
	logo := [][]Pixel{{{'#', cCyan, 0}}}
	frame, _ := composeFrame(logo, info, "SAM@PC", 100, 28, cfg)
	for _, expected := range []string{"Disk", "- C:\\Windows", "- D:\\example", "- E:\\example2"} {
		if !strings.Contains(frame, expected) {
			t.Fatalf("frame missing %q", expected)
		}
	}
}

func TestAdaptiveMinimumIsClampedToOne(t *testing.T) {
	cfg, err := parseYAMLConfig([]byte("adaptive_min: 0.5\nadaptive_max: 1.4\n"), defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdaptiveMin != 1 {
		t.Fatalf("adaptive_min must be clamped to 1, got %.2f", cfg.AdaptiveMin)
	}
}

func TestLegacyDefaultFieldsGainNewInfoWithoutConfigRewrite(t *testing.T) {
	cfg := defaultConfig()
	cfg.Fields = []string{"os", "kernel", "uptime", "shell", "display", "terminal", "cpu", "gpu", "memory", "disk", "locale"}
	if !equalStrings(cfg.Fields, []string{"os", "kernel", "uptime", "shell", "display", "terminal", "cpu", "gpu", "memory", "disk", "locale"}) {
		t.Fatal("legacy stock list should be recognized")
	}
	cfg.Fields = []string{"os", "kernel", "uptime", "shell", "display", "terminal", "cpu", "gpu", "memory", "disk", "host", "packages", "bios", "board", "locale"}
	if !equalStrings(cfg.Fields, defaultConfig().Fields) {
		t.Fatal("new stock list mismatch")
	}
}
