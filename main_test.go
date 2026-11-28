package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
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
	if got := formatVolume(`C:\`, "Windows", "Local Disk"); got != `C (Windows)` {
		t.Fatalf("got %q", got)
	}
	if got := formatVolume(`D:\`, "", "Local Disk"); got != `D (Local Disk)` {
		t.Fatalf("got %q", got)
	}
}

func TestDiskUsageFormatting(t *testing.T) {
	gib := uint64(1024 * 1024 * 1024)
	got := formatDiskUsage(`C:\`, "Windows", 100*gib, 25*gib)
	want := "C (Windows): 75.0 GB/100.0 GB (75%)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := formatDiskUsage(`D:\`, "Example", 0, 0); got != "D (Example): 0.0 GB/0.0 GB (0%)" {
		t.Fatalf("zero-capacity formatting got %q", got)
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

func TestAdaptiveScalingEffectiveEnablement(t *testing.T) {
	cfg := defaultConfig()
	if !adaptiveScalingEnabled(cfg) {
		t.Fatal("adaptive scaling should be enabled for the default config")
	}
	cfg.Size = 1.5
	if !adaptiveScalingEnabled(cfg) {
		t.Fatal("base size 1.5 should not disable adaptive scaling")
	}
	cfg.Size = 1.5001
	if adaptiveScalingEnabled(cfg) {
		t.Fatal("base size greater than 1.5 should disable all adaptive scaling")
	}
	cfg.Size = 1.0
	cfg.AdaptiveScale = false
	if adaptiveScalingEnabled(cfg) {
		t.Fatal("adaptive_scale: false should disable adaptive scaling")
	}
}

func TestLargeBaseSizeUsesFixedLogoViewportHeight(t *testing.T) {
	cfg := defaultConfig()
	if got := effectiveLogoViewportHeight(64, cfg); got != 64 {
		t.Fatalf("adaptive mode should use the available viewport height, got %d", got)
	}
	cfg.Size = 1.5
	if got := effectiveLogoViewportHeight(64, cfg); got != 64 {
		t.Fatalf("base size 1.5 should keep adaptive viewport sizing, got %d", got)
	}
	cfg.Size = 1.6
	if got := effectiveLogoViewportHeight(64, cfg); got != cfg.Height {
		t.Fatalf("base size above 1.5 should use fixed height %d, got %d", cfg.Height, got)
	}
	cfg.Size = 1.0
	cfg.AdaptiveScale = false
	if got := effectiveLogoViewportHeight(64, cfg); got != cfg.Height {
		t.Fatalf("adaptive_scale: false should use fixed height %d, got %d", cfg.Height, got)
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

func TestAdaptiveScaleRespondsToWideWindows(t *testing.T) {
	cfg := defaultConfig()
	wide := adaptiveScale(240, 36, cfg)
	if wide <= 1.0 || wide > 1.5 {
		t.Fatalf("wide terminal should increase scale within cap, got %.2f", wide)
	}
	if got := adaptiveScale(120, 36, cfg); got != 1.0 {
		t.Fatalf("baseline terminal should remain at 1.0, got %.2f", got)
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

func TestAdaptiveScaleCapsAtOnePointFiveAndRespectsLargeUserScale(t *testing.T) {
	cfg := defaultConfig()
	cfg.AdaptiveMax = 9
	if got := adaptiveScale(1000, 300, cfg); got > 1.5 {
		t.Fatalf("adaptive scale exceeded 1.5: %.2f", got)
	}
	cfg.Size = 1.6
	if got := adaptiveScale(1000, 300, cfg); got != 1.0 {
		t.Fatalf("adaptive scale should be disabled for user size > 1.5, got %.2f", got)
	}
}

func TestDiskInfoUsesMultilineList(t *testing.T) {
	got := buildInfo(nil, []string{"disk"}, NativeInfo{Disks: []string{`C:\Windows`, `D:\example`}})
	if len(got) != 1 || got[0].Label != "Disk" || len(got[0].Lines) != 2 {
		t.Fatalf("unexpected disk info: %#v", got)
	}
}

func TestVersionFlagIsDocumented(t *testing.T) {
	if !strings.Contains(version, "0.5.2") {
		t.Fatalf("unexpected W1F3 version: %s", version)
	}
}

func TestDiskListRendersOnSeparateBulletRows(t *testing.T) {
	cfg := defaultConfig()
	info := []InfoLine{{Label: "Disk", Lines: []string{"C (Windows): 75.0 GB/100.0 GB (75%)", "D (Example): 20.0 GB/50.0 GB (40%)"}}}
	logo := [][]Pixel{{{'#', cCyan, 0}}}
	frame, _ := composeFrame(logo, info, "SAM@PC", 100, 28, cfg)
	for _, expected := range []string{"Disk:", "C (Windows):", "D (Example):"} {
		if !strings.Contains(frame, expected) {
			t.Fatalf("frame missing %q", expected)
		}
	}
}

func TestDiskRowsUseTabWidthIndent(t *testing.T) {
	cfg := defaultConfig()
	info := []InfoLine{{Label: "Disk", Lines: []string{"C (Windows): 75 GB/100 GB (75%)"}}}
	logo := [][]Pixel{{{'#', cCyan, 0}}}
	frame, _ := composeFrame(logo, info, "SAM@PC", 100, 28, cfg)
	lines := strings.Split(frame, "\r\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "C (Windows):") {
			if !strings.Contains(line, "    \x1b[38;5;252mC (Windows):") {
				t.Fatalf("disk row should use four-column indentation, got %q", line)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("disk row not rendered")
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

func stripANSIForTest(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e) {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func TestComposeFrameCentersFixedHeightLogoCanvas(t *testing.T) {
	cfg := defaultConfig()
	logo := make([][]Pixel, 32)
	for y := range logo {
		logo[y] = make([]Pixel, 40)
		for x := range logo[y] {
			logo[y][x] = Pixel{'#', cCyan, 0}
		}
	}

	for _, dims := range [][2]int{{120, 40}, {120, 60}, {160, 60}} {
		cols, rows := dims[0], dims[1]
		frame, hitBounds := composeFrame(logo, nil, "", cols, rows, cfg)
		viewport := computeLayout(cols, rows, cfg.ShowInfo).Logo
		if hitBounds != viewport {
			t.Fatalf("mouse hit area should remain the full logo viewport at %dx%d: got %#v want %#v", cols, rows, hitBounds, viewport)
		}
		placed := centeredContentRect(viewport, len(logo[0]), len(logo))
		lines := strings.Split(stripANSIForTest(frame), "\r\n")
		firstY, firstX := -1, -1
		for y, line := range lines {
			if x := strings.IndexByte(line, '#'); x >= 0 {
				firstY, firstX = y, x
				break
			}
		}
		if firstY != placed.Y || firstX != placed.X {
			t.Fatalf("logo top-left at %dx%d = (%d,%d), want (%d,%d); viewport=%#v", cols, rows, firstX, firstY, placed.X, placed.Y, viewport)
		}
	}
}

func TestCenteredContentRectTracksViewportEveryFrame(t *testing.T) {
	contentW, contentH := 36, 24
	first := Rect{X: 10, Y: 1, W: 50, H: 38}
	second := Rect{X: 15, Y: 1, W: 70, H: 58}
	firstBounds := centeredContentRect(first, contentW, contentH)
	secondBounds := centeredContentRect(second, contentW, contentH)
	if firstBounds == secondBounds {
		t.Fatal("logo bounds should be recomputed when the terminal viewport changes")
	}
	if firstBounds.X != first.X+(first.W-contentW)/2 || firstBounds.Y != first.Y+(first.H-contentH)/2 {
		t.Fatalf("first viewport alignment incorrect: %#v", firstBounds)
	}
	if secondBounds.X != second.X+(second.W-contentW)/2 || secondBounds.Y != second.Y+(second.H-contentH)/2 {
		t.Fatalf("second viewport alignment incorrect: %#v", secondBounds)
	}
}

func TestFastfetchQueryUsesOnlySelectedNonNativeFields(t *testing.T) {
	got := fastfetchFieldsForConfig([]string{
		"os", "kernel", "uptime", "shell", "disk", "battery", "wifi", "cpu", "battery",
	})
	want := []string{"os", "battery", "wifi", "cpu"}
	if !equalStrings(got, want) {
		t.Fatalf("Fastfetch fields = %#v, want %#v", got, want)
	}
	if got := fastfetchFieldsForConfig([]string{"disk", "kernel", "uptime", "shell"}); len(got) != 0 {
		t.Fatalf("native-only fields must not be queried from Fastfetch: %#v", got)
	}
}

func TestConfigRejectsUnknownAndDuplicateFields(t *testing.T) {
	if _, err := parseYAMLConfig([]byte("fields:\n  - os\n  - not_a_module\n"), defaultConfig()); err == nil {
		t.Fatal("unknown information field should be rejected")
	}
	if _, err := parseYAMLConfig([]byte("fields:\n  - os\n  - os\n"), defaultConfig()); err == nil {
		t.Fatal("duplicate information field should be rejected")
	}
}

func TestDefaultConfigAndSchemaExposeEverySupportedField(t *testing.T) {
	configBytes, err := os.ReadFile("config.yaml")
	if err != nil { t.Fatal(err) }
	firstLine := strings.SplitN(string(configBytes), "\n", 2)[0]
	firstLine = strings.TrimSuffix(firstLine, "\r")
	if firstLine != "# yaml-language-server: $schema=https://raw.githubusercontent.com/sambui-Electron/W1F3/main/config.schema.json" {
		t.Fatalf("config.yaml schema directive is incorrect: %q", firstLine)
	}
	cfg, err := parseYAMLConfig(configBytes, defaultConfig())
	if err != nil { t.Fatalf("ported config is invalid: %v", err) }
	if len(cfg.Modules) != 29 { t.Fatalf("ported module sequence has %d entries, want 29", len(cfg.Modules)) }
	wantFields := []string{"host","cpu","gpu","memory","disk","os","kernel","bios","packages","shell","de","lm","wm","wmtheme","terminal","uptime","datetime"}
	if !equalStrings(cfg.Fields, wantFields) { t.Fatalf("active module fields differ from the source profile: %#v", cfg.Fields) }

	var schema struct {
		ID string `json:"$id"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	schemaBytes, err := os.ReadFile("config.schema.json")
	if err != nil { t.Fatal(err) }
	if err := json.Unmarshal(schemaBytes, &schema); err != nil { t.Fatalf("invalid JSON schema: %v", err) }
	const schemaURL = "https://raw.githubusercontent.com/sambui-Electron/W1F3/main/config.schema.json"
	if schema.ID != schemaURL { t.Fatalf("schema $id = %q, want %q", schema.ID, schemaURL) }

	var fieldsProperty struct { Items struct { Enum []string `json:"enum"` } `json:"items"` }
	if err := json.Unmarshal(schema.Properties["fields"], &fieldsProperty); err != nil { t.Fatal(err) }
	schemaFields := append([]string(nil), fieldsProperty.Items.Enum...)
	runtimeFields := append([]string(nil), supportedInfoFields...)
	sort.Strings(schemaFields)
	sort.Strings(runtimeFields)
	if !equalStrings(schemaFields, runtimeFields) {
		t.Fatalf("schema field enum does not match runtime catalog:\nschema: %#v\nruntime: %#v", schemaFields, runtimeFields)
	}

	var modulesProperty struct {
		Items struct {
			OneOf []struct {
				Properties struct { Type struct { Enum []string `json:"enum"` } `json:"type"` } `json:"properties"`
			} `json:"oneOf"`
		} `json:"items"`
	}
	if err := json.Unmarshal(schema.Properties["modules"], &modulesProperty); err != nil { t.Fatal(err) }
	if len(modulesProperty.Items.OneOf) != 2 { t.Fatalf("module schema must support break and module objects") }
	moduleEnums := append([]string(nil), modulesProperty.Items.OneOf[1].Properties.Type.Enum...)
	wantModuleEnums := []string{"break","custom","command","colors"}
	for _, field := range supportedInfoFields { if field != "colors" { wantModuleEnums = append(wantModuleEnums, field) } }
	sort.Strings(moduleEnums)
	sort.Strings(wantModuleEnums)
	if !equalStrings(moduleEnums, wantModuleEnums) {
		t.Fatalf("schema module types do not match runtime support:\nschema: %#v\nruntime: %#v", moduleEnums, wantModuleEnums)
	}
}

func TestGenericFormatterHandlesAdditionalFastfetchShapes(t *testing.T) {
	got := generic(map[string]any{"capacity": float64(82), "status": "Charging", "modelName": "Example Battery"})
	if got == "" {
		t.Fatal("generic formatter should emit useful output for module data outside the original field set")
	}
	fallback := generic(map[string]any{"driverInfo": "Adapter Driver", "firmwareRevision": "1.2.3"})
	if fallback == "" || !strings.Contains(fallback, "Adapter Driver") {
		t.Fatalf("generic map fallback lost scalar information: %q", fallback)
	}
}

func TestPortedFastfetchYAMLPreservesOriginalProfile(t *testing.T) {
	data, err := os.ReadFile("config.yaml")
	if err != nil { t.Fatal(err) }
	cfg, err := parseYAMLConfig(data, defaultConfig())
	if err != nil { t.Fatal(err) }
	if cfg.LogoFile != "C:/Users/PC/.config/fastfetch/ascii.txt" {
		t.Fatalf("logo source changed: %q", cfg.LogoFile)
	}
	if cfg.Separator != " " { t.Fatalf("separator = %q, want one space", cfg.Separator) }
	if cfg.LogoPaddingTop != 1 || cfg.LogoPaddingRight != 3 {
		t.Fatalf("logo padding lost: top=%d right=%d", cfg.LogoPaddingTop, cfg.LogoPaddingRight)
	}
	if len(cfg.LogoPalette) != 9 || cfg.LogoPalette["7"] != "#94E2D5" || cfg.LogoPalette["9"] != "#74C7EC" {
		t.Fatalf("logo palette changed: %#v", cfg.LogoPalette)
	}
	wantTypes := []string{"break","custom","host","cpu","gpu","memory","disk","custom","break","custom","os","kernel","bios","packages","shell","break","de","lm","wm","wmtheme","terminal","custom","break","custom","command","uptime","datetime","custom","colors"}
	if len(cfg.Modules) != len(wantTypes) { t.Fatalf("module count = %d, want %d", len(cfg.Modules), len(wantTypes)) }
	for i, want := range wantTypes {
		if cfg.Modules[i].Type != want { t.Fatalf("module[%d] = %q, want %q", i, cfg.Modules[i].Type, want) }
	}
	if cfg.Modules[2].Key != "󰌢  PC" || cfg.Modules[2].KeyColor != "green" {
		t.Fatalf("host presentation lost: %#v", cfg.Modules[2])
	}
	if cfg.Modules[24].Shell != "powershell" || !strings.Contains(cfg.Modules[24].Text, "Win32_OperatingSystem") {
		t.Fatalf("OS age command was not ported for Windows: %#v", cfg.Modules[24])
	}
	if cfg.Modules[28].PaddingLeft != 18 || cfg.Modules[28].Symbol != "circle" {
		t.Fatalf("terminal palette options lost: %#v", cfg.Modules[28])
	}
	if !cfg.Mouse || !cfg.AdaptiveScale || cfg.Speed != 0.45 {
		t.Fatalf("omitted W1F3 settings should retain application defaults: %#v", cfg)
	}
}

func TestLogoPaletteExpandsFastfetchColorSlots(t *testing.T) {
	got := applyLogoPalette("A$1B$7C$9D", map[string]string{
		"1":"#CDD6F4", "7":"#94E2D5", "9":"#74C7EC",
	})
	for _, expected := range []string{
		"\x1b[38;2;205;214;244m",
		"\x1b[38;2;148;226;213m",
		"\x1b[38;2;116;199;236m",
	} {
		if !strings.Contains(got, expected) { t.Fatalf("missing %q in %q", expected, got) }
	}
}

func TestConfiguredInfoPreservesFastfetchKeysAndColors(t *testing.T) {
	cfg := defaultConfig()
	cfg.Separator = " "
	cfg.Modules = []ModuleConfig{
		{Type:"custom", Format:"\x1b[90mHardware"},
		{Type:"host", Key:"󰌢  PC", KeyColor:"green"},
		{Type:"cpu", Key:"│ ├󰻠 ", KeyColor:"green"},
		{Type:"break"},
	}
	mods := map[string]any{
		"host": map[string]any{"name":"Workstation"},
		"cpu": map[string]any{"cpu":"Test CPU"},
	}
	info := buildConfiguredInfo(mods, cfg, NativeInfo{})
	if len(info) != 4 { t.Fatalf("got %d info rows: %#v", len(info), info) }
	if info[0].Kind != "custom" || info[0].Value != "Hardware" || info[0].ValueColor != cDim {
		t.Fatalf("custom section formatting lost: %#v", info[0])
	}
	if info[1].Label != "󰌢  PC" || info[1].LabelColor != "\x1b[32m" || info[1].Value != "Workstation" {
		t.Fatalf("host module presentation lost: %#v", info[1])
	}
	if info[2].Label != "│ ├󰻠 " || info[2].Value != "Test CPU" {
		t.Fatalf("CPU module presentation lost: %#v", info[2])
	}
	if info[3].Kind != "blank" { t.Fatalf("break module not preserved: %#v", info[3]) }
}

func TestYAMLParserRejectsUnsupportedModuleOptions(t *testing.T) {
	_, err := parseYAMLConfig([]byte("modules:\n  - type: cpu\n    unknownOption: true\n"), defaultConfig())
	if err == nil { t.Fatal("expected unsupported module option to be rejected") }
}

func TestYAMLParserHonorsExplicitFalseAndZeroValues(t *testing.T) {
	cfg, err := parseYAMLConfig([]byte("mouse: false\nadaptive_scale: false\nspeed: 0\n"), defaultConfig())
	if err != nil { t.Fatal(err) }
	if cfg.Mouse || cfg.AdaptiveScale || cfg.Speed != 0 {
		t.Fatalf("explicit false/zero values were not honored: %#v", cfg)
	}
}
