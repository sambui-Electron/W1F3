package main

import (
	"bufio"
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
	"strconv"
	"strings"
	"time"
	"unicode"
)

const version = "0.5.0"
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
}

func defaultConfig() Config {
	return Config{
		Logo: "Windows 11", Refresh: 1.0, Speed: 0.45, Size: 1.0,
		Depth: 1.0, Spin: "xy", Height: 32, AdaptiveScale: true, AdaptiveStrength: 0.65, AdaptiveMin: 1.0, AdaptiveMax: 1.45, Mouse: true,
		Color: true, ShowInfo: true, Box: false,
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

// parseYAMLConfig intentionally supports the scalar/list subset used by Fetch's
// config. It keeps the executable dependency-free while accepting ordinary YAML
// comments, quoted values, booleans, numbers and block lists.
func parseYAMLConfig(data []byte, base Config) (Config, error) {
	cfg := base
	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), " \t\r")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || trim == "---" || trim == "..." {
			continue
		}
		if strings.HasPrefix(trim, "- ") {
			if section != "fields" {
				return cfg, fmt.Errorf("config line %d: list item outside fields", lineNo)
			}
			v := unquoteYAML(stripYAMLComment(strings.TrimSpace(strings.TrimPrefix(trim, "- "))))
			if v != "" {
				cfg.Fields = append(cfg.Fields, strings.ToLower(v))
			}
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return cfg, fmt.Errorf("config line %d: unsupported nested YAML value", lineNo)
		}
		pair := strings.SplitN(trim, ":", 2)
		if len(pair) != 2 {
			return cfg, fmt.Errorf("config line %d: expected key: value", lineNo)
		}
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		raw := strings.TrimSpace(stripYAMLComment(strings.TrimSpace(pair[1])))
		if raw == "" {
			if key == "fields" {
				cfg.Fields = nil
				section = "fields"
				continue
			}
			return cfg, fmt.Errorf("config line %d: empty value for %s", lineNo, key)
		}
		section = ""
		value := unquoteYAML(raw)
		parseFloat := func() (float64, error) { return strconv.ParseFloat(value, 64) }
		parseBool := func() (bool, error) { return strconv.ParseBool(strings.ToLower(value)) }
		parseInt := func() (int, error) { n, e := strconv.Atoi(value); return n, e }
		var err error
		switch key {
		case "logo":
			cfg.Logo = value
		case "logo_file":
			cfg.LogoFile = value
		case "fastfetch_path":
			cfg.Fastfetch = value
		case "refresh":
			cfg.Refresh, err = parseFloat()
		case "speed":
			cfg.Speed, err = parseFloat()
		case "size":
			cfg.Size, err = parseFloat()
		case "depth":
			cfg.Depth, err = parseFloat()
		case "spin":
			cfg.Spin = strings.ToLower(value)
		case "height":
			cfg.Height, err = parseInt()
		case "adaptive_scale":
			cfg.AdaptiveScale, err = parseBool()
		case "adaptive_strength":
			cfg.AdaptiveStrength, err = parseFloat()
		case "adaptive_min":
			cfg.AdaptiveMin, err = parseFloat()
		case "adaptive_max":
			cfg.AdaptiveMax, err = parseFloat()
		case "mouse":
			cfg.Mouse, err = parseBool()
		case "color":
			cfg.Color, err = parseBool()
		case "show_info":
			cfg.ShowInfo, err = parseBool()
		case "box":
			cfg.Box, err = parseBool()
		case "fields":
			cfg.Fields = nil
			for _, item := range strings.Split(value, ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					cfg.Fields = append(cfg.Fields, strings.ToLower(unquoteYAML(item)))
				}
			}
		default:
			return cfg, fmt.Errorf("config line %d: unknown key %q", lineNo, key)
		}
		if err != nil {
			return cfg, fmt.Errorf("config line %d (%s): %w", lineNo, key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}
	if len(cfg.Fields) == 0 {
		cfg.Fields = []string{}
	}
	if cfg.Refresh < 0.2 {
		cfg.Refresh = 0.2
	}
	if cfg.Speed < 0 {
		cfg.Speed = 0
	}
	if cfg.Size < 0.2 {
		cfg.Size = 0.2
	}
	if cfg.Depth < 0.1 {
		cfg.Depth = 0.1
	}
	if cfg.Height < 0 {
		cfg.Height = 0
	}
	if cfg.AdaptiveStrength < 0 {
		cfg.AdaptiveStrength = 0
	}
	if cfg.AdaptiveMin < 1.0 {
		cfg.AdaptiveMin = 1.0
	}
	if cfg.AdaptiveMax < cfg.AdaptiveMin {
		cfg.AdaptiveMax = cfg.AdaptiveMin
	}
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

func fetchModules(exe string) (map[string]any, error) {
	out, err := runFastfetch(exe, "-c", "none", "-s", "os:display:terminal:cpu:gpu:memory:locale:host:packages:bios:board", "--format", "json")
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
		return drive
	}
	return drive + name
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
		for _, k := range []string{"prettyName", "name", "model", "gpu", "description", "value", "version", "release"} {
			if a, ok := x[k]; ok && a != nil && a != "" {
				if s := str(a); s != "" {
					return s
				}
			}
		}
		return ""
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

func titleCase(s string) string {
	if s == "os" {
		return "OS"
	}
	if s == "gpu" {
		return "GPU"
	}
	if s == "cpu" {
		return "CPU"
	}
	if s == "ip" {
		return "IP"
	}
	if s == "wm" {
		return "WM"
	}
	if s == "de" {
		return "DE"
	}
	if s == "localip" {
		return "Local IP"
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
	layout := computeLayout(cols, rows, cfg.ShowInfo)
	for y, row := range logo {
		yy := layout.Logo.Y + y
		if yy < 0 || yy >= rows {
			continue
		}
		for x, px := range row {
			xx := layout.Logo.X + x
			if xx >= layout.Logo.X && xx < layout.Logo.X+layout.Logo.W && xx < cols {
				canvas[yy][xx] = px
			}
		}
	}
	if cfg.ShowInfo && layout.InfoW > 12 {
		infoW := layout.InfoW
		contentX := layout.InfoX
		contentRows := 0
		for _, line := range info {
			if len(line.Lines) > 0 {
				contentRows += 1 + len(line.Lines)
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
		keyW := minInt(11, maxInt(5, infoW/4))
		valueW := infoW - keyW - 2
		for _, line := range info {
			if y >= rows {
				break
			}
			if len(line.Lines) > 0 {
				put(canvas, contentX, y, clip(line.Label, infoW), cCyan, infoW)
				y++
				for _, item := range line.Lines {
					if y >= rows {
						break
					}
					put(canvas, contentX+1, y, clip("- "+item, infoW-1), cWhite, infoW-1)
					y++
				}
				continue
			}
			put(canvas, contentX, y, clip(line.Label, keyW), cCyan, keyW)
			put(canvas, contentX+keyW, y, ": ", cDim, 2)
			put(canvas, contentX+keyW+2, y, clip(line.Value, valueW), cWhite, valueW)
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

func adaptiveScale(cols, rows int, cfg Config) float64 {
	if !cfg.AdaptiveScale {
		return 1
	}
	// Baseline is a medium terminal (120 columns x 36 rows).
	widthFactor := float64(cols) / 120
	heightFactor := float64(rows) / 36
	base := math.Min(widthFactor, heightFactor)
	multiplier := 1 + (base-1)*cfg.AdaptiveStrength
	return math.Max(1.0, math.Min(cfg.AdaptiveMax, multiplier))
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
	mods, err := fetchModules(ff)
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
			if fresh, e := fetchModules(ff); e == nil {
				mods = fresh
			}
			if lastRefresh.IsZero() {
				time.Sleep(300 * time.Millisecond)
			}
			native := readNativeInfo()
			info = buildInfo(mods, cfg.Fields, native)
			lastRefresh = now
		}
		cols, rows := terminalSize()
		if rows < 8 {
			rows = 8
		}
		if cols < 32 {
			cols = 32
		}
		layout := computeLayout(cols, rows, cfg.ShowInfo)
		viewportW := maxInt(8, layout.Logo.W)
		viewportH := maxInt(6, layout.Logo.H)
		if !cfg.AdaptiveScale && cfg.Height > 0 && cfg.Height < viewportH {
			viewportH = cfg.Height
		}
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
		return parseLogo(string(b)), nil
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
	rows := parseLogo(string(out))
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

func fatal(err error) { fmt.Fprintln(os.Stderr, "fetch:", err); os.Exit(1) }
func printHelp() {
	fmt.Printf("W1F3 %s\n\nUsage: fetch [options]\n\n  --config PATH     YAML config (default ~/.config/fetch/config.yaml)\n  --logo NAME       Fastfetch logo name (cached in ~/.config/fetch/logos/)\n  --logo-file PATH  Custom ANSI/ASCII logo file\n  --fastfetch PATH  Fastfetch executable path\n  --speed FLOAT     Rotation speed\n  --size FLOAT      Logo scale\n  --depth FLOAT     3D extrusion depth\n  --no-info         Hide system information\n  --no-color        Disable colors\n  --frames N        Stop after N rendered frames\n  -v, --version     Print version\n\nMouse: click and drag the logo to rotate; release to fling.\nKeyboard: Q / Esc / Enter exits; +/- changes spin speed.\n", version)
}
