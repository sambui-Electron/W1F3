package main

import (
	"context"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

const version = "0.5.2"
const defaultConfigURL = "https://raw.githubusercontent.com/sambui-Electron/W1F3/main/config.yaml"

var downloadDefaultConfig = func() ([]byte, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, defaultConfigURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "W1F3-Fetch/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download default config: GitHub returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("downloaded default config is empty")
	}
	return data, nil
}

const defaultShading = ".,-~:;=!*#$@"

const (
	cReset = "\x1b[0m"
	cCyan  = "\x1b[38;5;81m"
	cWhite = "\x1b[38;5;252m"
	cDim   = "\x1b[38;5;240m"
)

type ModuleConfig struct {
	Type         string `yaml:"type"`
	Key          string `yaml:"key"`
	KeyColor     string `yaml:"keyColor"`
	Format       string `yaml:"format"`
	Text         string `yaml:"text"`
	Shell        string `yaml:"shell"`
	PaddingLeft  int    `yaml:"paddingLeft"`
	Symbol       string `yaml:"symbol"`
}

type Config struct {
	Logo             string
	LogoFile         string
	Fastfetch        string
	Refresh          float64
	Speed            float64
	Size             float64
	Depth            float64
	Spin             string
	Height           int
	AdaptiveScale    bool
	AdaptiveStrength float64
	AdaptiveMin      float64
	AdaptiveMax      float64
	Mouse            bool
	Color            bool
	ShowInfo         bool
	Box              bool
	Fields           []string
	Modules          []ModuleConfig
	Separator        string
	LogoPalette      map[string]string
	LogoPaddingTop   int
	LogoPaddingRight int
}

// supportedInfoFields lists the information modules W1F3 can display.
// Presentation modules (break, custom, command, colors) are parsed separately
// and rendered directly by W1F3 rather than requested from Fastfetch.
var supportedInfoFields = []string{
	"battery", "bios", "bluetooth", "bluetoothradio", "board", "bootmgr",
	"brightness", "btrfs", "camera", "chassis", "codec", "cpu", "cpucache", "cpuusage",
	"colors", "cursor", "datetime", "display", "disk", "diskio", "de", "dns",
	"editor", "font", "gamepad", "gpu", "host", "icons", "initsystem",
	"keyboard", "kernel", "lm", "loadavg", "locale", "localip", "media",
	"memory", "monitor", "mouse", "netio", "opencl", "opengl", "os", "packages",
	"physicaldisk", "physicalmemory", "player", "poweradapter", "processes",
	"publicip", "shell", "sound", "swap", "terminal", "terminalfont", "top",
	"terminalsize", "terminaltheme", "theme", "tpm", "uptime", "users", "version",
	"vulkan", "wallpaper", "weather", "wm", "wifi", "wmtheme", "zpool",
}

var nativeInfoFields = map[string]bool{
	"disk": true, "kernel": true, "shell": true, "uptime": true, "colors": true,
}

func isSupportedInfoField(field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	for _, supported := range supportedInfoFields {
		if field == supported {
			return true
		}
	}
	return false
}

// fastfetchFieldsForConfig returns only modules that should be queried from
// Fastfetch. Disk, kernel, shell and uptime are collected by W1F3's native
// Windows code and therefore do not belong in the Fastfetch structure request.
func fastfetchFieldsForConfig(fields []string) []string {
	result := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if !isSupportedInfoField(field) || nativeInfoFields[field] || seen[field] {
			continue
		}
		seen[field] = true
		result = append(result, field)
	}
	return result
}

func defaultConfig() Config {
	return Config{
		Logo: "Windows 11", Refresh: 1.0, Speed: 0.45, Size: 1.0,
		Depth: 1.0, Spin: "xy", Height: 32, AdaptiveScale: true, AdaptiveStrength: 0.65, AdaptiveMin: 1.0, AdaptiveMax: 1.5, Mouse: true,
		Color: true, ShowInfo: true, Box: false, Separator: ": ",
		Fields: []string{"os", "kernel", "uptime", "shell", "display", "terminal", "cpu", "gpu", "memory", "disk", "host", "packages", "bios", "board", "locale"},
	}
}

func configRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".config", "fetch")
}

func configPath() string { return filepath.Join(configRoot(), "config.yaml") }

// yamlDocument is the typed top-level W1F3 YAML contract. Nested module and logo
// nodes are validated explicitly so unsupported Fastfetch options fail clearly.
type yamlDocument struct {
	Logo             yaml.Node `yaml:"logo"`
	LogoFile         string    `yaml:"logo_file"`
	FastfetchPath    string    `yaml:"fastfetch_path"`
	Refresh          *float64  `yaml:"refresh"`
	Speed            *float64  `yaml:"speed"`
	Size             *float64  `yaml:"size"`
	Depth            *float64  `yaml:"depth"`
	Spin             string    `yaml:"spin"`
	Height           *int      `yaml:"height"`
	AdaptiveScale    *bool     `yaml:"adaptive_scale"`
	AdaptiveStrength *float64  `yaml:"adaptive_strength"`
	AdaptiveMin      *float64  `yaml:"adaptive_min"`
	AdaptiveMax      *float64  `yaml:"adaptive_max"`
	Mouse            *bool     `yaml:"mouse"`
	Color            *bool     `yaml:"color"`
	ShowInfo         *bool     `yaml:"show_info"`
	Box              *bool     `yaml:"box"`
	Fields           []string  `yaml:"fields"`
	Modules          []yaml.Node `yaml:"modules"`
	Display          struct { Separator string `yaml:"separator"` } `yaml:"display"`
	Separator        string    `yaml:"separator"`
	LogoPalette      map[string]string `yaml:"logo_palette"`
	LogoPadding      struct {
		Top int `yaml:"top"`
		Right int `yaml:"right"`
	} `yaml:"logo_padding"`
}

func parseYAMLConfig(data []byte, base Config) (Config, error) {
	cfg := base
	var doc yamlDocument
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return cfg, fmt.Errorf("parse YAML config: %w", err)
	}
	cfg.LogoFile = doc.LogoFile
	cfg.Fastfetch = doc.FastfetchPath
	if doc.Refresh != nil { cfg.Refresh = *doc.Refresh }
	if doc.Speed != nil { cfg.Speed = *doc.Speed }
	if doc.Size != nil { cfg.Size = *doc.Size }
	if doc.Depth != nil { cfg.Depth = *doc.Depth }
	if doc.Spin != "" { cfg.Spin = strings.ToLower(doc.Spin) }
	if doc.Height != nil { cfg.Height = *doc.Height }
	if doc.AdaptiveScale != nil { cfg.AdaptiveScale = *doc.AdaptiveScale }
	if doc.AdaptiveStrength != nil { cfg.AdaptiveStrength = *doc.AdaptiveStrength }
	if doc.AdaptiveMin != nil { cfg.AdaptiveMin = *doc.AdaptiveMin }
	if doc.AdaptiveMax != nil { cfg.AdaptiveMax = *doc.AdaptiveMax }
	if doc.Mouse != nil { cfg.Mouse = *doc.Mouse }
	if doc.Color != nil { cfg.Color = *doc.Color }
	if doc.ShowInfo != nil { cfg.ShowInfo = *doc.ShowInfo }
	if doc.Box != nil { cfg.Box = *doc.Box }
	if doc.Separator != "" { cfg.Separator = doc.Separator }
	if doc.Display.Separator != "" { cfg.Separator = doc.Display.Separator }
	if doc.Logo.Kind != 0 {
		switch doc.Logo.Kind {
		case yaml.ScalarNode:
			cfg.Logo = doc.Logo.Value
		case yaml.MappingNode:
			var logo struct {
				Type string `yaml:"type"`
				Source string `yaml:"source"`
				Color map[string]string `yaml:"color"`
				Padding struct { Top int `yaml:"top"`; Right int `yaml:"right"` } `yaml:"padding"`
			}
			for j := 0; j < len(doc.Logo.Content); j += 2 {
				key := doc.Logo.Content[j].Value
				if key != "type" && key != "source" && key != "color" && key != "padding" {
					return cfg, fmt.Errorf("logo: unknown property %q", key)
				}
			}
			if err := doc.Logo.Decode(&logo); err != nil { return cfg, fmt.Errorf("logo: %w", err) }
			if logo.Type != "file" { return cfg, fmt.Errorf("unsupported logo type %q; expected file", logo.Type) }
			if strings.TrimSpace(logo.Source) == "" { return cfg, errors.New("logo.source is required when logo.type is file") }
			if logo.Padding.Top < 0 || logo.Padding.Right < 0 { return cfg, errors.New("logo padding cannot be negative") }
			if len(doc.Logo.Content) > 0 {
				for j := 0; j < len(doc.Logo.Content); j += 2 {
					if doc.Logo.Content[j].Value == "padding" {
						paddingNode := doc.Logo.Content[j+1]
						for k := 0; k < len(paddingNode.Content); k += 2 {
							name := paddingNode.Content[k].Value
							if name != "top" && name != "right" { return cfg, fmt.Errorf("logo.padding: unknown property %q", name) }
						}
					}
				}
			}
			cfg.LogoFile = logo.Source
			cfg.LogoPalette = logo.Color
			cfg.LogoPaddingTop, cfg.LogoPaddingRight = logo.Padding.Top, logo.Padding.Right
		default:
			return cfg, errors.New("logo must be a string or mapping")
		}
	}
	if len(doc.LogoPalette) > 0 { cfg.LogoPalette = doc.LogoPalette }
	if doc.LogoPadding.Top < 0 || doc.LogoPadding.Right < 0 { return cfg, errors.New("logo_padding values cannot be negative") }
	if doc.LogoPadding.Top != 0 { cfg.LogoPaddingTop = doc.LogoPadding.Top }
	if doc.LogoPadding.Right != 0 { cfg.LogoPaddingRight = doc.LogoPadding.Right }
	if len(doc.Modules) > 0 {
		cfg.Modules = nil
		cfg.Fields = nil
		for i, node := range doc.Modules {
			module := ModuleConfig{}
			if node.Kind == yaml.ScalarNode {
				module.Type = strings.ToLower(strings.TrimSpace(node.Value))
			} else if node.Kind == yaml.MappingNode {
				for j := 0; j < len(node.Content); j += 2 {
					key := node.Content[j].Value
					switch key {
					case "type", "key", "keyColor", "format", "text", "shell", "paddingLeft", "symbol":
					default: return cfg, fmt.Errorf("modules[%d]: unknown property %q", i, key)
					}
				}
				if err := node.Decode(&module); err != nil { return cfg, fmt.Errorf("modules[%d]: %w", i, err) }
			} else {
				return cfg, fmt.Errorf("modules[%d] must be a string or mapping", i)
			}
			module.Type = strings.ToLower(strings.TrimSpace(module.Type))
			if module.Type == "" { return cfg, fmt.Errorf("modules[%d]: missing type", i) }
			if module.PaddingLeft < 0 { return cfg, fmt.Errorf("modules[%d]: paddingLeft cannot be negative", i) }
			if module.KeyColor != "" {
				switch strings.ToLower(module.KeyColor) {
				case "gray", "grey", "green", "yellow", "blue", "magenta", "cyan", "red", "white":
				default: return cfg, fmt.Errorf("modules[%d]: unsupported keyColor %q", i, module.KeyColor)
				}
			}
			switch module.Type {
			case "break", "custom", "command", "colors":
				if module.Type == "custom" && module.Format == "" { return cfg, fmt.Errorf("modules[%d]: custom module requires format", i) }
				if module.Type == "command" {
					if module.Text == "" { return cfg, fmt.Errorf("modules[%d]: command module requires text", i) }
					if module.Shell == "" { module.Shell = "powershell" }
					if module.Shell != "powershell" && module.Shell != "pwsh" && module.Shell != "cmd" { return cfg, fmt.Errorf("modules[%d]: unsupported command shell %q", i, module.Shell) }
				}
				if module.Type == "colors" && module.Symbol != "" && module.Symbol != "circle" && module.Symbol != "square" { return cfg, fmt.Errorf("modules[%d]: symbol must be circle or square", i) }
			default:
				if !isSupportedInfoField(module.Type) { return cfg, fmt.Errorf("modules[%d]: unsupported module type %q", i, module.Type) }
				cfg.Fields = append(cfg.Fields, module.Type)
			}
			cfg.Modules = append(cfg.Modules, module)
		}
	} else if doc.Fields != nil {
		cfg.Fields = make([]string, 0, len(doc.Fields))
		for _, field := range doc.Fields { cfg.Fields = append(cfg.Fields, strings.ToLower(strings.TrimSpace(field))) }
	}
	for key, color := range cfg.LogoPalette {
		if len(key) != 1 || key[0] < '1' || key[0] > '9' || len(color) != 7 || color[0] != '#' {
			return cfg, fmt.Errorf("invalid logo color slot %q: expected slots 1-9 and #RRGGBB values", key)
		}
		if _, err := strconv.ParseUint(color[1:], 16, 32); err != nil { return cfg, fmt.Errorf("invalid logo color for slot %s: %w", key, err) }
	}
	seen := make(map[string]bool, len(cfg.Fields))
	for _, field := range cfg.Fields {
		if !isSupportedInfoField(field) { return cfg, fmt.Errorf("unsupported information field %q; see config.schema.json for available fields", field) }
		if len(cfg.Modules) == 0 && seen[field] { return cfg, fmt.Errorf("duplicate information field %q", field) }
		seen[field] = true
	}
	if cfg.Refresh < 0.2 { cfg.Refresh = 0.2 }
	if cfg.Speed < 0 { cfg.Speed = 0 }
	if cfg.Size < 0.2 { cfg.Size = 0.2 }
	if cfg.Depth < 0.1 { cfg.Depth = 0.1 }
	if cfg.Height < 0 { cfg.Height = 0 }
	if cfg.AdaptiveStrength < 0 { cfg.AdaptiveStrength = 0 }
	if cfg.AdaptiveStrength > 1 { cfg.AdaptiveStrength = 1 }
	if cfg.AdaptiveMin < 1 { cfg.AdaptiveMin = 1 }
	if cfg.AdaptiveMax < cfg.AdaptiveMin { cfg.AdaptiveMax = cfg.AdaptiveMin }
	if cfg.AdaptiveMax > 1.5 { cfg.AdaptiveMax = 1.5 }
	return cfg, nil
}

func stripYAMLComment(s string) string {
	quote := rune(0)
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && quote == '"' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			continue
		}
		if r == '#' && (i == 0 || unicode.IsSpace(rune(s[i-1]))) {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}

func unquoteYAML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		if s[0] == '"' {
			if v, err := strconv.Unquote(s); err == nil {
				return v
			}
		}
		return s[1 : len(s)-1]
	}
	return s
}

func loadConfig(path string) (Config, string, error) {
	cfg := defaultConfig()
	if path == "" {
		path = configPath()
	} else {
		path = expandPath(path)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
			return cfg, path, mkErr
		}
		data, err = downloadDefaultConfig()
		if err != nil {
			return cfg, path, fmt.Errorf("first-run config download failed from %s: %w\nCheck your internet connection, or create %s manually", defaultConfigURL, err, path)
		}
		if _, parseErr := parseYAMLConfig(data, defaultConfig()); parseErr != nil {
			return cfg, path, fmt.Errorf("downloaded default config is invalid: %w", parseErr)
		}
		if writeErr := os.WriteFile(path, data, 0644); writeErr != nil {
			return cfg, path, writeErr
		}
	} else if err != nil {
		return cfg, path, err
	}
	cfg, err = parseYAMLConfig(data, cfg)
	if err != nil {
		return cfg, path, err
	}
	// Upgrade the previous stock field list in memory, without rewriting the
	// user's config.yaml. Custom field selections remain untouched.
	legacyFields := []string{"os", "kernel", "uptime", "shell", "display", "terminal", "cpu", "gpu", "memory", "disk", "locale"}
	if equalStrings(cfg.Fields, legacyFields) {
		cfg.Fields = []string{"os", "kernel", "uptime", "shell", "display", "terminal", "cpu", "gpu", "memory", "disk", "host", "packages", "bios", "board", "locale"}
	}
	return cfg, path, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func expandPath(path string) string {
	path = os.ExpandEnv(path)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, strings.TrimLeft(path[1:], `/\\`))
	}
	return path
}

func findFastfetch(configured string) (string, error) {
	if env := os.Getenv("FASTFETCH_EXE"); env != "" {
		configured = env
	}
	if configured != "" {
		configured = expandPath(configured)
		if filepath.IsAbs(configured) {
			if _, err := os.Stat(configured); err == nil {
				return configured, nil
			}
		}
		if p, err := exec.LookPath(configured); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("Fastfetch executable not found: %s", configured)
	}
	for _, name := range []string{"fastfetch.exe", "fastfetch"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("fastfetch.exe was not found on PATH; install Fastfetch or set fastfetch_path in config.yaml")
}

func runFastfetch(exe string, args ...string) ([]byte, error) {
	cmd := exec.Command(exe, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Fastfetch: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type FastfetchModule struct {
	Type   string `json:"type"`
	Result any    `json:"result"`
	Error  string `json:"error"`
}

func decodeFastfetchJSON(raw []byte) (map[string]any, error) {
	var modules []FastfetchModule
	if err := json.Unmarshal(raw, &modules); err != nil {
		return nil, fmt.Errorf("parse Fastfetch JSON: %w", err)
	}
	result := make(map[string]any)
	for _, m := range modules {
		if m.Type != "" && m.Result != nil {
			result[strings.ToLower(m.Type)] = m.Result
		}
	}
	return result, nil
}

func fetchModules(exe string, fields []string) (map[string]any, error) {
	requested := fastfetchFieldsForConfig(fields)
	if len(requested) == 0 {
		return map[string]any{}, nil
	}
	out, err := runFastfetch(exe, "-c", "none", "-s", strings.Join(requested, ":"), "--format", "json")
	if err != nil {
		return nil, err
	}
	return decodeFastfetchJSON(out)
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func value(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil && v != "" {
			return v
		}
	}
	return nil
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case bool:
		if x {
			return "Yes"
		}
		return "No"
	case []any:
		parts := make([]string, 0, len(x))
		for _, a := range x {
			if s := str(a); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		for _, k := range []string{"name", "cpu", "gpu", "prettyName", "description", "model", "product", "value", "version"} {
			if v, ok := x[k]; ok && v != nil && v != "" {
				return str(v)
			}
		}
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func fmtBytes(v any) string {
	f, ok := number(v)
	if !ok || f < 0 {
		return str(v)
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", f, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	case json.Number:
		f, e := x.Float64()
		return f, e == nil
	case string:
		f, e := strconv.ParseFloat(x, 64)
		return f, e == nil
	default:
		return 0, false
	}
}

func formatVolume(drive, label, fallback string) string {
	_ = fallback
	name := strings.TrimSpace(label)
	if name == "" {
		name = "Local Disk"
	}
	letter := strings.TrimSuffix(strings.TrimRight(drive, `\/`), ":")
	return letter + " (" + name + ")"
}

func formatDiskUsage(drive, label string, totalBytes, freeBytes uint64) string {
	name := strings.TrimSpace(label)
	if name == "" {
		name = "Local Disk"
	}
	used := uint64(0)
	if totalBytes > freeBytes {
		used = totalBytes - freeBytes
	}
	const gib = float64(1024 * 1024 * 1024)
	usedGB := float64(used) / gib
	totalGB := float64(totalBytes) / gib
	percent := 0.0
	if totalBytes > 0 {
		percent = float64(used) * 100 / float64(totalBytes)
	}
	letter := strings.TrimSuffix(strings.TrimRight(drive, `\/`), ":")
	return fmt.Sprintf("%s (%s): %s GB/%s GB (%.0f%%)", letter, name, compactNumber(usedGB), compactNumber(totalGB), percent)
}

func compactNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func fmtUptime(seconds uint64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	parts = append(parts, fmt.Sprintf("%dm", mins))
	return strings.Join(parts, " ")
}

func fmtCPU(m map[string]any) string {
	// Fastfetch's current CPU JSON property is `cpu`, not `name`.
	name := str(value(m, "cpu", "name", "model", "brand"))
	cores, _ := m["cores"].(map[string]any)
	physical := str(value(cores, "physical"))
	logical := str(value(cores, "logical"))
	parts := []string{}
	if name != "" {
		parts = append(parts, name)
	}
	if physical != "" {
		detail := physical + " cores"
		if logical != "" && logical != physical {
			detail += ", " + logical + " threads"
		}
		parts = append(parts, detail)
	}
	return strings.Join(parts, " · ")
}

func fmtMemory(m map[string]any) string {
	used := value(m, "used", "usedBytes")
	total := value(m, "total", "totalBytes")
	if used == nil || total == nil {
		return str(m)
	}
	uf, uok := number(used)
	tf, tok := number(total)
	if !uok || !tok || tf <= 0 {
		return str(m)
	}
	return fmt.Sprintf("%s / %s (%.0f%%)", fmtBytes(used), fmtBytes(total), uf/tf*100)
}

func fmtDisplay(v any) string {
	list, ok := v.([]any)
	if !ok {
		list = []any{v}
	}
	var items []string
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			if s := str(item); s != "" {
				items = append(items, s)
			}
			continue
		}
		name := str(value(m, "name", "display", "model"))
		w := str(value(m, "width", "currentWidth", "resolutionWidth"))
		h := str(value(m, "height", "currentHeight", "resolutionHeight"))
		res := ""
		if w != "" && h != "" {
			res = w + "x" + h
		}
		if hz := str(value(m, "refreshRate", "refresh", "frequency")); hz != "" {
			res += " @ " + hz + "Hz"
		}
		v := strings.TrimSpace(name + " " + res)
		if v != "" {
			items = append(items, v)
		}
	}
	return strings.Join(items, "; ")
}

func fmtTerminal(v any) string {
	if m := asMap(v); m != nil {
		name := str(value(m, "name", "terminal", "processName"))
		version := str(value(m, "version", "terminalVersion"))
		if name != "" {
			lower := strings.ToLower(name)
			switch lower {
			case "windowsterminal.exe", "windows terminal":
				name = "Windows Terminal"
			case "conhost.exe", "console window host":
				name = "Console Host"
			case "pwsh.exe":
				name = "PowerShell 7"
			case "powershell.exe":
				name = "Windows PowerShell"
			case "cmd.exe":
				name = "Command Prompt"
			}
			if version != "" {
				return name + " · " + version
			}
			return name
		}
		if name != "" {
			return name
		}
	}
	return generic(v)
}

func generic(v any) string {
	switch x := v.(type) {
	case map[string]any:
		// Prefer human-readable identity/status properties, then fall back to
		// scalar fields so less common Fastfetch modules are still displayable.
		preferred := []string{
			"prettyName", "name", "model", "product", "vendor", "manufacturer",
			"cpu", "gpu", "description", "ssid", "ip", "address", "percent",
			"percentage", "capacity", "status", "state", "value", "version",
			"release", "temperature", "frequency", "used", "total", "available", "size",
		}
		parts := make([]string, 0, 4)
		used := make(map[string]bool, 4)
		for _, key := range preferred {
			if a, ok := x[key]; ok && a != nil && a != "" {
				if value := str(a); value != "" && !used[value] {
					parts = append(parts, value)
					used[value] = true
					if len(parts) == 4 {
						break
					}
				}
			}
		}
		if len(parts) == 0 {
			keys := make([]string, 0, len(x))
			for key := range x {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				value := str(x[key])
				if value == "" || strings.HasPrefix(value, "map[") || strings.HasPrefix(value, "[") {
					continue
				}
				parts = append(parts, titleCase(key)+": "+value)
				if len(parts) == 4 {
					break
				}
			}
		}
		return strings.Join(parts, " · ")
	case []any:
		items := []string{}
		for _, a := range x {
			if s := generic(a); s != "" {
				items = append(items, s)
			}
		}
		return strings.Join(items, "; ")
	default:
		return str(v)
	}
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

type LogoCell struct {
	Char  rune
	Color string
}
type Point struct {
	X, Y, Z, NX, NY, NZ float64
	Color               string
}
type Pixel struct {
	Char  rune
	Color string
	Z     float64
}
type Rect struct{ X, Y, W, H int }

type DragState struct {
	Active               bool
	LastX, LastY         int
	VelocityX, VelocityY float64
}

func (d *DragState) Update(mouse MouseState, bounds Rect, angleX, angleY *float64) {
	if mouse.Left && !d.Active {
		inside := mouse.X >= bounds.X && mouse.X < bounds.X+bounds.W && mouse.Y >= bounds.Y && mouse.Y < bounds.Y+bounds.H
		if inside {
			d.Active = true
			d.LastX, d.LastY = mouse.X, mouse.Y
			d.VelocityX, d.VelocityY = 0, 0
		}
	}
	if d.Active && mouse.Moved {
		dx, dy := mouse.X-d.LastX, mouse.Y-d.LastY
		*angleX += float64(dy) * 0.018
		*angleY += float64(dx) * 0.018
		d.VelocityX, d.VelocityY = float64(dy)*0.018, float64(dx)*0.018
		d.LastX, d.LastY = mouse.X, mouse.Y
	}
	if d.Active && !mouse.Left {
		d.Active = false
	}
}

func parseLogo(data string) [][]LogoCell {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	rows := make([][]LogoCell, 0, len(lines))
	for _, line := range lines {
		row := []LogoCell{}
		color := ""
		for len(line) > 0 {
			if strings.HasPrefix(line, "\x1b[") {
				if end := strings.IndexByte(line, 'm'); end >= 0 {
					seq := line[:end+1]
					params := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
					parts := strings.Split(params, ";")
					reset := len(parts) == 0
					foreground := false
					for _, part := range parts {
						n, _ := strconv.Atoi(part)
						if n == 0 || n == 39 {
							reset = true
						}
						if (n >= 30 && n <= 37) || (n >= 90 && n <= 97) || n == 38 {
							foreground = true
						}
					}
					if foreground {
						color = seq
					} else if reset {
						color = ""
					}
					line = line[end+1:]
					continue
				}
			}
			r, size := rune(line[0]), 1
			if r >= 0x80 {
				r, size = decodeRune(line)
			}
			line = line[size:]
			if unicode.Is(unicode.Mn, r) {
				continue
			}
			row = append(row, LogoCell{r, color})
			if isWide(r) {
				row = append(row, LogoCell{' ', color})
			}
		}
		rows = append(rows, row)
	}
	for len(rows) > 0 {
		occupied := false
		for _, c := range rows[len(rows)-1] {
			if c.Char != ' ' {
				occupied = true
				break
			}
		}
		if occupied {
			break
		}
		rows = rows[:len(rows)-1]
	}
	return rows
}

func decodeRune(s string) (rune, int) {
	for n := 2; n <= 4 && n <= len(s); n++ {
		r := []rune(s[:n])
		if len(r) == 1 && r[0] != unicode.ReplacementChar {
			return r[0], n
		}
	}
	return rune(s[0]), 1
}

func isWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || (r >= 0x2E80 && r <= 0xA4CF) || (r >= 0xAC00 && r <= 0xD7A3) || (r >= 0xF900 && r <= 0xFAFF) || (r >= 0xFE10 && r <= 0xFE6F) || (r >= 0xFF00 && r <= 0xFF60) || (r >= 0x1F300 && r <= 0x1FAFF)
}

func charWeight(r rune) float64 {
	weights := map[rune]float64{'M': 1, 'N': .88, 'm': .76, 'd': .66, 'h': .56, 'b': .56, 'y': .46, 'o': .38, 'n': .38, 's': .30, '+': .22, ':': .18, '=': .22, '-': .14, '`': .08, '.': .10, '/': .12, '\'': .06, ' ': 0, '█': 1, '▓': .75, '▒': .5, '░': .25, '▀': .5, '▄': .5, '▌': .5, '▐': .5}
	if v, ok := weights[r]; ok {
		return v
	}
	if unicode.IsUpper(r) {
		return .8
	}
	if unicode.IsLower(r) {
		return .5
	}
	if unicode.IsDigit(r) {
		return .4
	}
	return .35
}

func makePoints(rows [][]LogoCell, depth float64) ([]Point, int, int) {
	h := len(rows)
	w := 0
	for _, row := range rows {
		if len(row) > w {
			w = len(row)
		}
	}
	occupied := map[[2]int]LogoCell{}
	for y, row := range rows {
		for x, cell := range row {
			if charWeight(cell.Char) > .015 {
				occupied[[2]int{x, y}] = cell
			}
		}
	}
	points := []Point{}
	for pos, cell := range occupied {
		x, y := pos[0], pos[1]
		weight := charWeight(cell.Char)
		px := float64(x) - float64(w-1)/2
		py := (float64(y) - float64(h-1)/2) * 2
		thick := (.20 + weight*.95) * depth
		front := thick / 2
		points = append(points, Point{px, py, front, 0, 0, 1, cell.Color}, Point{px, py, -front, 0, 0, -1, cell.Color})
		sides := []struct {
			pos            [2]int
			nx, ny, ox, oy float64
		}{{[2]int{x - 1, y}, -1, 0, -.5, 0}, {[2]int{x + 1, y}, 1, 0, .5, 0}, {[2]int{x, y - 1}, 0, -1, 0, -1}, {[2]int{x, y + 1}, 0, 1, 0, 1}}
		steps := maxInt(2, int(2+depth*2))
		for _, side := range sides {
			if _, ok := occupied[side.pos]; ok {
				continue
			}
			for n := 0; n <= steps; n++ {
				z := -front + 2*front*float64(n)/float64(steps)
				points = append(points, Point{px + side.ox, py + side.oy, z, side.nx, side.ny, 0, cell.Color})
			}
		}
	}
	return points, w, h
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func rotate(p Point, ax, ay float64) (x, y, z, nx, ny, nz float64) {
	cx, sx := math.Cos(ax), math.Sin(ax)
	cy, sy := math.Cos(ay), math.Sin(ay)
	y1 := p.Y*cx - p.Z*sx
	z1 := p.Y*sx + p.Z*cx
	x = p.X*cy + z1*sy
	z = -p.X*sy + z1*cy
	y = y1
	ny1 := p.NY*cx - p.NZ*sx
	nz1 := p.NY*sx + p.NZ*cx
	nx = p.NX*cy + nz1*sy
	nz = -p.NX*sy + nz1*cy
	ny = ny1
	return
}

func renderLogo(points []Point, sourceW, sourceH, width, height int, ax, ay, scale float64, useColor bool) ([][]Pixel, Rect) {
	canvas := make([][]Pixel, height)
	for y := range canvas {
		canvas[y] = make([]Pixel, width)
		for x := range canvas[y] {
			canvas[y][x] = Pixel{' ', "", math.Inf(-1)}
		}
	}
	if len(points) == 0 || width < 2 || height < 2 {
		return canvas, Rect{}
	}
	fitX := float64(width-4) / math.Max(1, float64(sourceW)*1.8)
	fitY := float64(height-3) / math.Max(1, float64(sourceH)*1.5)
	fit := math.Min(fitX, fitY) * scale
	radius := math.Max(float64(sourceW), float64(sourceH)*2) * fit
	camera := math.Max(14, radius*3.0)
	lightLen := math.Sqrt(1.0 + 1.0 + .25)
	lightX, lightY, lightZ := -1/lightLen, -1/lightLen, 0.5/lightLen
	for _, p := range points {
		x, y, z, nx, ny, nz := rotate(p, ax, ay)
		perspective := camera / (camera - z)
		col := int(math.Round(float64(width)/2 + x*fit*perspective))
		row := int(math.Round(float64(height)/2 + y*fit*.5*perspective))
		if col < 0 || col >= width || row < 0 || row >= height {
			continue
		}
		if z < canvas[row][col].Z {
			continue
		}
		dot := math.Max(0, nx*lightX+ny*lightY+nz*lightZ)
		viewX, viewY, viewZ := -x, -y, camera-z
		viewLen := math.Sqrt(viewX*viewX + viewY*viewY + viewZ*viewZ)
		if viewLen > 0 {
			viewX /= viewLen
			viewY /= viewLen
			viewZ /= viewLen
		}
		hx, hy, hz := lightX+viewX, lightY+viewY, lightZ+viewZ
		hLen := math.Sqrt(hx*hx + hy*hy + hz*hz)
		spec := 0.0
		if hLen > 0 {
			hx /= hLen
			hy /= hLen
			hz /= hLen
			spec = math.Pow(math.Max(0, nx*hx+ny*hy+nz*hz), 18)
		}
		intensity := .18 + .72*dot + .25*spec
		idx := int(intensity * float64(len(defaultShading)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(defaultShading) {
			idx = len(defaultShading) - 1
		}
		color := p.Color
		if useColor && color == "" {
			color = "\x1b[38;5;81m"
		}
		if !useColor {
			color = ""
		}
		canvas[row][col] = Pixel{rune(defaultShading[idx]), color, z}
	}
	return canvas, Rect{0, 0, width, height}
}

func put(canvas [][]Pixel, x, y int, text, color string, maxWidth int) {
	if y < 0 || y >= len(canvas) || maxWidth <= 0 {
		return
	}
	col := x
	used := 0
	for _, r := range text {
		if used >= maxWidth || col+used >= len(canvas[y]) {
			break
		}
		canvas[y][col+used] = Pixel{r, color, math.Inf(1)}
		if isWide(r) && col+used+1 < len(canvas[y]) {
			canvas[y][col+used+1] = Pixel{' ', color, math.Inf(1)}
			used++
		}
		used++
	}
}

func clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
	if len(r) > width {
		if width == 1 {
			return "…"
		}
		return string(r[:width-1]) + "…"
	}
	return string(r)
}

type InfoLine struct {
	Label, Value string
	Lines        []string
	Kind         string
	LabelColor   string
	ValueColor   string
	Separator    string
	PaddingLeft  int
	Symbol       string
}

func buildInfo(mods map[string]any, fields []string, native NativeInfo) []InfoLine {
	result := []InfoLine{}
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field))
		var v string
		switch key {
		case "os":
			v = generic(value(asMap(mods["os"]), "prettyName", "name", "productName"))
		case "kernel":
			v = "Win32_NT"
		case "uptime":
			v = fmtUptime(native.UptimeSeconds)
		case "shell":
			v = native.Shell
		case "cpu":
			v = fmtCPU(asMap(mods["cpu"]))
			if v == "" {
				v = cpuNameFallback()
			}
		case "disk":
			lines := native.Disks
			if len(lines) > 0 {
				infoLine := InfoLine{Label: titleCase(key), Lines: lines}
				result = append(result, infoLine)
				continue
			}
		case "memory":
			v = fmtMemory(asMap(mods["memory"]))
		case "display":
			v = fmtDisplay(mods["display"])
		case "gpu":
			v = generic(mods["gpu"])
		case "terminal":
			v = fmtTerminal(mods["terminal"])
			if v == "" {
				v = native.Terminal
			}
		case "locale":
			v = generic(mods["locale"])
		case "host":
			v = generic(mods["host"])
		case "packages":
			v = generic(mods["packages"])
		default:
			v = generic(mods[key])
		}
		if v != "" {
			result = append(result, InfoLine{Label: titleCase(key), Value: v})
		}
	}
	return result
}

func buildConfiguredInfo(mods map[string]any, cfg Config, native NativeInfo) []InfoLine {
	if len(cfg.Modules) == 0 { return buildInfo(mods, cfg.Fields, native) }
	result := make([]InfoLine, 0, len(cfg.Modules))
	for _, module := range cfg.Modules {
		switch module.Type {
		case "break":
			result = append(result, InfoLine{Kind: "blank"})
		case "custom":
			text, color := stripInfoANSI(module.Format)
			result = append(result, InfoLine{Kind: "custom", Value: text, ValueColor: color, PaddingLeft: module.PaddingLeft})
		case "colors":
			result = append(result, InfoLine{Kind: "palette", PaddingLeft: module.PaddingLeft, Symbol: module.Symbol})
		case "command":
			value, err := runConfiguredCommand(module.Shell, module.Text)
			if err != nil || strings.TrimSpace(value) == "" { continue }
			label := module.Key
			if label == "" { label = "Command" }
			result = append(result, InfoLine{Label: label, Value: strings.TrimSpace(value), LabelColor: moduleKeyColor(module.KeyColor), ValueColor: cWhite, Separator: cfg.Separator})
		default:
			lines := buildInfo(mods, []string{module.Type}, native)
			if len(lines) == 0 { continue }
			line := lines[0]
			if module.Key != "" { line.Label = module.Key }
			line.LabelColor = moduleKeyColor(module.KeyColor)
			line.ValueColor = cWhite
			line.Separator = cfg.Separator
			result = append(result, line)
		}
	}
	return result
}

func moduleKeyColor(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gray", "grey": return cDim
	case "green": return "\x1b[32m"
	case "yellow": return "\x1b[33m"
	case "blue": return "\x1b[34m"
	case "magenta": return "\x1b[35m"
	case "cyan": return "\x1b[36m"
	case "red": return "\x1b[31m"
	case "white": return cWhite
	default: return cCyan
	}
}

func stripInfoANSI(input string) (string, string) {
	color := cWhite
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] == 0x1b && i+1 < len(input) && input[i+1] == '[' {
			end := strings.IndexByte(input[i+2:], 'm')
			if end >= 0 {
				seq := input[i+2:i+2+end]
				if seq == "90" || strings.HasPrefix(seq, "90;") { color = cDim }
				if seq == "0" { color = cWhite }
				i += end + 3
				continue
			}
		}
		out.WriteByte(input[i])
		i++
	}
	return out.String(), color
}

type commandCacheEntry struct {
	value string
	at time.Time
}

var commandCacheState = struct {
	sync.Mutex
	entries map[string]commandCacheEntry
}{entries: make(map[string]commandCacheEntry)}

func runConfiguredCommand(shell, command string) (string, error) {
	cacheKey := shell + "\x00" + command
	commandCacheState.Lock()
	if entry, ok := commandCacheState.entries[cacheKey]; ok && time.Since(entry.at) < time.Minute {
		commandCacheState.Unlock()
		return entry.value, nil
	}
	commandCacheState.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch strings.ToLower(shell) {
	case "powershell":
		name := "powershell.exe"
		if _, err := exec.LookPath(name); err != nil { name = "pwsh" }
		cmd = exec.CommandContext(ctx, name, "-NoProfile", "-NonInteractive", "-Command", command)
	case "pwsh":
		cmd = exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-Command", command)
	case "cmd":
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	default:
		return "", fmt.Errorf("unsupported command shell %q", shell)
	}
	out, err := cmd.Output()
	if err != nil { return "", fmt.Errorf("configured command failed: %w", err) }
	value := strings.TrimSpace(string(out))
	commandCacheState.Lock()
	commandCacheState.entries[cacheKey] = commandCacheEntry{value: value, at: time.Now()}
	commandCacheState.Unlock()
	return value, nil
}

func applyLogoPalette(data string, palette map[string]string) string {
	for slot, hex := range palette {
		if len(hex) != 7 || hex[0] != '#' { continue }
		rgb, err := strconv.ParseUint(hex[1:], 16, 32)
		if err != nil { continue }
		sequence := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", (rgb>>16)&255, (rgb>>8)&255, rgb&255)
		data = strings.ReplaceAll(data, "$"+slot, sequence)
	}
	return data
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		if isWide(r) { width += 2 } else { width++ }
	}
	return width
}

func computeLayoutForConfig(cols, rows int, cfg Config) Layout {
	layout := computeLayout(cols, rows, cfg.ShowInfo)
	if cfg.ShowInfo && cfg.LogoPaddingRight > 0 {
		layout.InfoX += cfg.LogoPaddingRight
		layout.InfoW = maxInt(0, layout.InfoW-cfg.LogoPaddingRight)
	}
	return layout
}

func titleCase(s string) string {
	labels := map[string]string{
		"os": "OS", "gpu": "GPU", "cpu": "CPU", "ip": "IP", "wm": "WM", "de": "DE",
		"localip": "Local IP", "publicip": "Public IP", "bluetoothradio": "Bluetooth Radio",
		"bootmgr": "Boot Manager", "cpucache": "CPU Cache", "cpuusage": "CPU Usage",
		"diskio": "Disk I/O", "initsystem": "Init System", "loadavg": "Load Average",
		"netio": "Network I/O", "opencl": "OpenCL", "opengl": "OpenGL",
		"physicaldisk": "Physical Disk", "physicalmemory": "Physical Memory",
		"poweradapter": "Power Adapter", "terminalfont": "Terminal Font",
		"terminalsize": "Terminal Size", "terminaltheme": "Terminal Theme", "codec": "Video Codecs",
		"top":     "Top Processes",
		"wmtheme": "WM Theme", "wifi": "Wi-Fi", "tpm": "TPM",
	}
	if label, ok := labels[s]; ok {
		return label
	}
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type Layout struct {
	Logo         Rect
	InfoX, InfoW int
}

func computeLayout(cols, rows int, showInfo bool) Layout {
	if !showInfo {
		return Layout{Logo: Rect{X: 0, Y: 1, W: cols, H: rows - 2}}
	}
	// Keep the logo and information panel as one centered composition.
	panelW := minInt(54, maxInt(36, int(float64(cols)*0.36)))
	gap := maxInt(3, minInt(7, cols/40))
	logoW := minInt(maxInt(30, int(float64(cols)*0.40)), cols-panelW-gap-4)
	if logoW < 18 {
		logoW = maxInt(8, cols-panelW-gap-2)
	}
	groupW := logoW + gap + panelW
	startX := maxInt(0, (cols-groupW)/2)
	return Layout{
		Logo:  Rect{X: startX, Y: 1, W: logoW, H: rows - 2},
		InfoX: startX + logoW + gap,
		InfoW: minInt(panelW, cols-(startX+logoW+gap)-1),
	}
}

// centeredContentRect positions a rendered content canvas inside its layout
// viewport. Recompute it for every frame so terminal resizes and fixed-height
// logo canvases stay aligned with the information panel.
func centeredContentRect(viewport Rect, contentW, contentH int) Rect {
	visibleW := minInt(maxInt(0, contentW), viewport.W)
	visibleH := minInt(maxInt(0, contentH), viewport.H)
	return Rect{
		X: viewport.X + maxInt(0, (viewport.W-visibleW)/2),
		Y: viewport.Y + maxInt(0, (viewport.H-visibleH)/2),
		W: visibleW,
		H: visibleH,
	}
}

func composeFrame(logo [][]Pixel, info []InfoLine, userHost string, cols, rows int, cfg Config) (string, Rect) {
	if cols < 32 {
		cols = 32
	}
	if rows < 8 {
		rows = 8
	}
	canvas := make([][]Pixel, rows)
	for y := range canvas {
		canvas[y] = make([]Pixel, cols)
		for x := range canvas[y] {
			canvas[y][x] = Pixel{' ', "", math.Inf(-1)}
		}
	}
	layout := computeLayoutForConfig(cols, rows, cfg)
	logoBounds := layout.Logo
	if len(logo) > 0 {
		logoWidth := len(logo[0])
		logoBounds = centeredContentRect(layout.Logo, logoWidth, len(logo))
		logoBounds.Y += cfg.LogoPaddingTop
		for y, row := range logo {
			yy := logoBounds.Y + y
			if yy < layout.Logo.Y || yy >= layout.Logo.Y+layout.Logo.H || yy < 0 || yy >= rows {
				continue
			}
			for x, px := range row {
				xx := logoBounds.X + x
				if xx >= layout.Logo.X && xx < layout.Logo.X+layout.Logo.W && xx >= 0 && xx < cols {
					canvas[yy][xx] = px
				}
			}
		}
	}
	if cfg.ShowInfo && layout.InfoW > 12 {
		infoW := layout.InfoW
		contentX := layout.InfoX
		contentRows := 0
		for _, line := range info {
			if line.Kind == "blank" || line.Kind == "custom" || line.Kind == "palette" {
				contentRows++
			} else if len(line.Lines) > 0 {
				if line.Separator != "" { contentRows += len(line.Lines) } else { contentRows += 1 + len(line.Lines) }
			} else {
				contentRows++
			}
		}
		visible := contentRows + 2
		startY := maxInt(0, (rows-visible)/2)
		if cfg.Box {
			infoW = maxInt(8, infoW-2)
			contentX++
			top := maxInt(0, startY-1)
			bottom := minInt(rows-1, startY+contentRows+2)
			borderX := layout.InfoX
			right := minInt(cols-1, layout.InfoX+layout.InfoW-1)
			for x := borderX; x <= right; x++ {
				canvas[top][x] = Pixel{'─', cDim, math.Inf(1)}
				canvas[bottom][x] = Pixel{'─', cDim, math.Inf(1)}
			}
			canvas[top][borderX] = Pixel{'┌', cDim, math.Inf(1)}
			canvas[top][right] = Pixel{'┐', cDim, math.Inf(1)}
			canvas[bottom][borderX] = Pixel{'└', cDim, math.Inf(1)}
			canvas[bottom][right] = Pixel{'┘', cDim, math.Inf(1)}
			for y := top + 1; y < bottom; y++ {
				canvas[y][borderX] = Pixel{'│', cDim, math.Inf(1)}
				canvas[y][right] = Pixel{'│', cDim, math.Inf(1)}
			}
		}
		put(canvas, contentX, startY, clip(userHost, infoW), "\x1b[1;38;5;81m", infoW)
		put(canvas, contentX, startY+1, strings.Repeat("─", minInt(infoW, maxInt(12, len(userHost)))), cDim, infoW)
		y := startY + 2
		for _, line := range info {
			if y >= rows { break }
			if line.Kind == "blank" { y++; continue }
			if line.Kind == "custom" {
				x := contentX + line.PaddingLeft
				put(canvas, x, y, clip(line.Value, infoW-line.PaddingLeft), line.ValueColor, infoW-line.PaddingLeft)
				y++
				continue
			}
			if line.Kind == "palette" {
				x := contentX + line.PaddingLeft
				symbol := "●"
				if line.Symbol == "square" { symbol = "■" }
				for n := 0; n < 16 && x < contentX+infoW; n++ {
					put(canvas, x, y, symbol, fmt.Sprintf("\x1b[38;5;%dm", n), 1)
					x++
				}
				y++
				continue
			}
			if len(line.Lines) > 0 {
				labelColor := line.LabelColor
				if labelColor == "" { labelColor = cCyan }
				if line.Separator != "" {
					label := clip(line.Label, infoW)
					labelWidth := displayWidth(label)
					put(canvas, contentX, y, label, labelColor, infoW)
					put(canvas, contentX+labelWidth, y, line.Separator, cDim, displayWidth(line.Separator))
					valueX := contentX + labelWidth + displayWidth(line.Separator)
					put(canvas, valueX, y, clip(line.Lines[0], infoW-(valueX-contentX)), cWhite, infoW-(valueX-contentX))
					y++
					for _, item := range line.Lines[1:] {
						if y >= rows { break }
						put(canvas, contentX+4, y, clip(item, infoW-4), cWhite, infoW-4)
						y++
					}
				} else {
					put(canvas, contentX, y, clip(line.Label+":", infoW), labelColor, infoW)
					y++
					for _, item := range line.Lines {
						if y >= rows { break }
						put(canvas, contentX+4, y, clip(item, infoW-4), cWhite, infoW-4)
						y++
					}
				}
				continue
			}
			labelColor := line.LabelColor
			if labelColor == "" { labelColor = cCyan }
			valueColor := line.ValueColor
			if valueColor == "" { valueColor = cWhite }
			separator := line.Separator
			if separator == "" { separator = cfg.Separator }
			if separator == "" { separator = ": " }
			label := clip(line.Label, infoW)
			labelWidth := displayWidth(label)
			put(canvas, contentX, y, label, labelColor, infoW)
			put(canvas, contentX+labelWidth, y, separator, cDim, displayWidth(separator))
			valueX := contentX + labelWidth + displayWidth(separator)
			put(canvas, valueX, y, clip(line.Value, infoW-(valueX-contentX)), valueColor, infoW-(valueX-contentX))
			y++
		}
	}
	var b strings.Builder
	b.WriteString("\x1b[H")
	for rowIndex, row := range canvas {
		current := "\x00"
		for _, px := range row {
			color := px.Color
			if color == "" {
				color = cReset
			}
			if color != current {
				b.WriteString(color)
				current = color
			}
			b.WriteRune(px.Char)
		}
		b.WriteString(cReset + "\x1b[K")
		if rowIndex != len(canvas)-1 {
			b.WriteString("\r\n")
		}
	}
	return b.String(), layout.Logo
}

// adaptiveScalingEnabled reports whether the adaptive multiplier and
// responsive viewport-height behavior should be active.
func adaptiveScalingEnabled(cfg Config) bool {
	return cfg.AdaptiveScale && cfg.Size <= 1.5
}

func effectiveLogoViewportHeight(available int, cfg Config) int {
	if !adaptiveScalingEnabled(cfg) && cfg.Height > 0 && cfg.Height < available {
		return cfg.Height
	}
	return available
}

func adaptiveScale(cols, rows int, cfg Config) float64 {
	if !adaptiveScalingEnabled(cfg) {
		return 1
	}
	// A medium terminal (120x36) is the neutral baseline. Use the geometric
	// mean so extra width still contributes when the terminal is unusually wide,
	// while height continues to constrain the available rendering area.
	widthFactor := math.Max(1.0, float64(cols)/120.0)
	heightFactor := math.Max(1.0, float64(rows)/36.0)
	windowFactor := math.Sqrt(widthFactor * heightFactor)
	multiplier := 1.0 + (windowFactor-1.0)*math.Max(0, cfg.AdaptiveStrength)
	maxScale := math.Min(1.5, math.Max(1.0, cfg.AdaptiveMax))
	return math.Max(1.0, math.Min(maxScale, multiplier))
}

func main() {
	var cfgPath, logoArg, logoFileArg, fastfetchArg string
	var speedArg, sizeArg, depthArg float64
	var frames int
	var noInfo, noColor, showHelp, showVersion bool
	flag.StringVar(&cfgPath, "config", "", "YAML configuration path")
	flag.StringVar(&logoArg, "logo", "", "Fastfetch logo name")
	flag.StringVar(&logoFileArg, "logo-file", "", "Custom ASCII/ANSI logo file")
	flag.StringVar(&fastfetchArg, "fastfetch", "", "Fastfetch executable path")
	flag.Float64Var(&speedArg, "speed", -1, "rotation speed multiplier")
	flag.Float64Var(&sizeArg, "size", -1, "logo size multiplier")
	flag.Float64Var(&depthArg, "depth", -1, "3D depth multiplier")
	flag.IntVar(&frames, "frames", 0, "exit after N frames (diagnostics)")
	flag.BoolVar(&noInfo, "no-info", false, "hide system information")
	flag.BoolVar(&noColor, "no-color", false, "disable ANSI colors")
	flag.BoolVar(&showHelp, "help", false, "show help")
	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.BoolVar(&showVersion, "v", false, "show version")
	flag.Parse()
	if showHelp {
		printHelp()
		return
	}
	if showVersion {
		fmt.Println("W1F3", version)
		return
	}
	cfg, confPath, err := loadConfig(cfgPath)
	if err != nil {
		fatal(err)
	}
	if logoArg != "" {
		cfg.Logo = logoArg
		cfg.LogoFile = ""
	}
	if logoFileArg != "" {
		cfg.LogoFile = logoFileArg
	}
	if fastfetchArg != "" {
		cfg.Fastfetch = fastfetchArg
	}
	if speedArg >= 0 {
		cfg.Speed = speedArg
	}
	if sizeArg > 0 {
		cfg.Size = sizeArg
	}
	if depthArg > 0 {
		cfg.Depth = depthArg
	}
	if noInfo {
		cfg.ShowInfo = false
	}
	if noColor {
		cfg.Color = false
	}
	if cfg.Spin != "x" && cfg.Spin != "y" {
		cfg.Spin = "xy"
	}
	if err := os.MkdirAll(filepath.Join(configRoot(), "logos"), 0755); err != nil {
		fatal(err)
	}
	ff, err := findFastfetch(cfg.Fastfetch)
	if err != nil {
		fatal(err)
	}
	mods, err := fetchModules(ff, cfg.Fields)
	if err != nil {
		fatal(err)
	}
	logoRows, err := loadLogo(ff, cfg, confPath)
	if err != nil {
		fatal(err)
	}
	points, lw, lh := makePoints(logoRows, cfg.Depth)
	if len(points) == 0 {
		fatal(errors.New("logo is empty or contains no visible characters"))
	}
	termCleanup, err := startTerminal(cfg.Mouse)
	if err != nil {
		fatal(err)
	}
	defer termCleanup()
	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	defer fmt.Print("\x1b[0m\x1b[?25h\x1b[?1000l\x1b[?1002l\x1b[?1006l\x1b[?1049l")
	ax, ay := 0.20, 0.25
	drag := DragState{}
	lastRefresh := time.Time{}
	frame := 0
	info := []InfoLine{}
	userHost := currentUserHost()
	bounds := Rect{}
	for {
		now := time.Now()
		if lastRefresh.IsZero() || now.Sub(lastRefresh) >= time.Duration(cfg.Refresh*float64(time.Second)) {
			if fresh, e := fetchModules(ff, cfg.Fields); e == nil {
				mods = fresh
			}
			if lastRefresh.IsZero() {
				time.Sleep(300 * time.Millisecond)
			}
			native := readNativeInfo()
			info = buildConfiguredInfo(mods, cfg, native)
			lastRefresh = now
		}
		cols, rows := terminalSize()
		if rows < 8 {
			rows = 8
		}
		if cols < 32 {
			cols = 32
		}
		layout := computeLayoutForConfig(cols, rows, cfg)
		viewportW := maxInt(8, layout.Logo.W)
		viewportH := effectiveLogoViewportHeight(maxInt(6, layout.Logo.H), cfg)
		angleX, angleY := ax, ay
		if cfg.Spin == "x" {
			angleY = 0
		}
		if cfg.Spin == "y" {
			angleX = 0
		}
		logo, _ := renderLogo(points, lw, lh, maxInt(8, viewportW), maxInt(6, viewportH), angleX, angleY, cfg.Size*adaptiveScale(cols, rows, cfg), cfg.Color)
		out, b := composeFrame(logo, info, userHost, cols, rows, cfg)
		bounds = b
		_, _ = os.Stdout.WriteString(out)
		if cfg.Spin != "y" && !drag.Active {
			ax += cfg.Speed*.018 + drag.VelocityX
			drag.VelocityX *= .985
		}
		if cfg.Spin != "x" && !drag.Active {
			ay += cfg.Speed*.024 + drag.VelocityY
			drag.VelocityY *= .985
		}
		if math.Abs(drag.VelocityX) < .0001 {
			drag.VelocityX = 0
		}
		if math.Abs(drag.VelocityY) < .0001 {
			drag.VelocityY = 0
		}
		for _, ev := range pollEvents() {
			if ev.Key != "" {
				k := strings.ToLower(ev.Key)
				if k == "q" || k == "escape" || k == "enter" || k == " " {
					return
				}
				if k == "+" || k == "=" {
					cfg.Speed += .1
				}
				if k == "-" {
					cfg.Speed = math.Max(0, cfg.Speed-.1)
				}
			}
			if !cfg.Mouse || ev.Mouse == nil {
				continue
			}
			drag.Update(*ev.Mouse, bounds, &ax, &ay)
		}
		frame++
		if frames > 0 && frame >= frames {
			return
		}
		time.Sleep(33 * time.Millisecond)
	}
}

func loadLogo(ff string, cfg Config, configFile string) ([][]LogoCell, error) {
	if cfg.LogoFile != "" {
		p := expandPath(cfg.LogoFile)
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(configFile), p)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		return parseLogo(applyLogoPalette(string(b), cfg.LogoPalette)), nil
	}
	cache := filepath.Join(configRoot(), "logos", slug(cfg.Logo)+".txt")
	if b, e := os.ReadFile(cache); e == nil {
		rows := parseLogo(string(b))
		if len(rows) > 0 {
			return rows, nil
		}
	}
	out, err := runFastfetch(ff, "-c", "none", "-l", cfg.Logo, "-s", "break", "--pipe", "false")
	if err != nil {
		return nil, fmt.Errorf("could not load Fastfetch logo %q: %w", cfg.Logo, err)
	}
	rows := parseLogo(applyLogoPalette(string(out), cfg.LogoPalette))
	if len(rows) == 0 {
		return nil, errors.New("Fastfetch returned an empty logo; check the logo name with fastfetch --list-logos")
	}
	if e := os.MkdirAll(filepath.Dir(cache), 0755); e == nil {
		_ = os.WriteFile(cache, out, 0644)
	}
	return rows, nil
}

func currentUserHost() string {
	user := os.Getenv("USERNAME")
	if user == "" {
		user = os.Getenv("USER")
	}
	if user == "" {
		user = "user"
	}
	host := os.Getenv("COMPUTERNAME")
	if host == "" {
		host, _ = os.Hostname()
	}
	if host == "" {
		host = "Windows"
	}
	return user + "@" + host
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "W1F3:", err); os.Exit(1) }
func printHelp() {
	fmt.Printf("W1F3 %s\n\nUsage: W1F3 [options]\n\n  --config PATH     YAML config (default ~/.config/fetch/config.yaml)\n  --logo NAME       Fastfetch logo name (cached in ~/.config/fetch/logos/)\n  --logo-file PATH  Custom ANSI/ASCII logo file\n  --fastfetch PATH  Fastfetch executable path\n  --speed FLOAT     Rotation speed\n  --size FLOAT      Logo scale\n  --depth FLOAT     3D extrusion depth\n  --no-info         Hide system information\n  --no-color        Disable colors\n  --frames N        Stop after N rendered frames\n  -v, --version     Print version\n\nMouse: click and drag the logo to rotate; release to fling.\nKeyboard: Q / Esc / Enter exits; +/- changes spin speed.\n", version)
}
