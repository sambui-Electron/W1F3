# W1F3

**W1F3** (pronounced “Wife”) is a Windows-native, animated 3D system-information viewer inspired by [areofyl/fetch](https://github.com/areofyl/fetch) and powered by [Fastfetch](https://github.com/fastfetch-cli/fastfetch).

W1F3 is written in Go and targets Windows 10/11 x64. The primary executable and terminal command are `w1f3.exe` and `W1F3`; `fetch` remains as a compatibility alias.

## Features

- Animated 3D Fastfetch logo with mouse drag rotation and release momentum.
- Live system-information panel.
- Windows uptime, shell, terminal, CPU, GPU, memory and named-volume information.
- Responsive layout and adaptive logo scaling.
- User configuration at `%USERPROFILE%\.config\fetch\config.yaml`.
- First-run default configuration downloaded from this repository's `main/config.yaml` on GitHub.
- Linked JSON Schema for YAML editor validation and completion.
- Ordered Fastfetch-style YAML modules with per-module labels and colors, custom section rows, PowerShell command modules, blank-line breaks, and terminal color swatches. The information catalog supports 70 field names: 66 queried from Fastfetch, three collected natively by W1F3 (`disk`, `shell`, and `uptime`), and `colors` rendered natively. Logo and separator settings are also handled directly by W1F3.
- Cached logos stored under `%USERPROFILE%\.config\fetch\logos\`.
- `W1F3 -v` / `W1F3 --version` reports the installed version. `fetch` remains available as a backward-compatible alias.

## Requirements

- Windows 10 or 11, x64.
- Fastfetch installed and available as `fastfetch.exe` on `PATH`. Fastfetch 2.68.0 or newer is recommended for the complete selectable module catalog.
- Windows Terminal recommended.
- Go 1.22+ only when building from source. Go is not required to run the release executable.

## Install a release

1. Download and extract the release ZIP.
2. Open PowerShell in the extracted folder.
3. Run:

   ```powershell
   Set-ExecutionPolicy -Scope Process Bypass
   .\install.ps1
   ```

4. Restart the terminal and run:

   ```powershell
   W1F3 -v
   W1F3
   ```

The installer upgrades the existing installation in `%LOCALAPPDATA%\Programs\Fetch`, preserves the user's configuration settings and logo cache, refreshes the schema, repairs the YAML schema reference, and updates the user PATH. It does **not** replace `config.yaml`.

## First launch and remote default configuration

When W1F3 cannot find `%USERPROFILE%\.config\fetch\config.yaml`, it downloads the canonical default from:

```text
https://raw.githubusercontent.com/sambui-Electron/W1F3/main/config.yaml
```

The downloaded YAML is validated by W1F3's parser before being written locally. Its first-line YAML Language Server directive links directly to the repository's `config.schema.json`, so editors can load validation and completion even when the config was downloaded separately. The installer also places a local copy of the schema beside the config file. Existing user configuration is never replaced or silently refreshed. If GitHub cannot be reached on first launch, W1F3 reports the download error and leaves the configuration absent so the user can retry or create it manually.

**Maintainers:** `config.yaml` remains tracked in the GitHub repository because it is the remote default source. It is not embedded in the Go executable and is not included in release ZIPs. Update this file in `main` to change defaults for future first-time installations. Existing installations keep their local config.

## Configuration

Location:

```text
%USERPROFILE%\.config\fetch\config.yaml
```

Schema:

```text
%USERPROFILE%\.config\fetch\config.schema.json
```

The first line of `config.yaml` contains a YAML Language Server directive pointing to the matching schema in this repository. The installer also copies the matching schema beside the local config for local tooling, and refreshes that copy on upgrade. When upgrading an older config, the installer repairs only the schema-reference comment; it preserves all configuration values and field selections.

### Ordered Fastfetch-style modules

The `modules` list is rendered in the exact order written in YAML. It supports:

- A scalar `break` entry for a blank line.
- Information modules such as `host`, `cpu`, `gpu`, `memory`, `disk`, `os`, `kernel`, `bios`, `packages`, `shell`, `de`, `lm`, `wm`, `wmtheme`, `terminal`, `uptime`, and `datetime`.
- `key` and `keyColor` for the module label and its color.
- `custom` modules for section headings and separators.
- `command` modules for locally executed commands. Supported shells are PowerShell, PowerShell 7 (`pwsh`), and `cmd`. Commands have a three-second timeout and their output is cached for 60 seconds. Only use commands from configuration files you trust.
- `colors` for the 16 terminal-color swatches, with `paddingLeft` and `symbol` options.

The `display.separator` value is inserted between each module key and its value. The `logo` mapping supports a custom file source, numbered `$1`–`$9` true-color replacements, and top/right padding. W1F3 also retains its own flat animation settings (`refresh`, `speed`, `size`, `depth`, `spin`, `height`, adaptive scaling, mouse, color, info visibility, and box).

The supplied profile's OS-age command was ported to PowerShell and calculates the number of days since the Windows installation date. Its logo source is still the original machine-specific path; update `logo.source` if the file is stored elsewhere.

For simpler configurations, the legacy `fields` list and flat `logo_file` settings remain supported. The schema validates module names and options and provides YAML editor completion. W1F3 collects `disk`, `shell`, and `uptime` natively; the `kernel` module is queried from Fastfetch so it displays the actual Windows kernel release. Other information modules are also queried from Fastfetch. Modules that have no result on the current machine are omitted. Network-backed Fastfetch modules may perform network lookups.

## Logo cache

Runtime logo cache:

```text
%USERPROFILE%\.config\fetch\logos\
```

The repository-level `/logos/` directory is intentionally Git-ignored. Cached or user-generated logo files should not be committed.

## Build from source

```powershell
.\build.ps1
```

Or directly:

```powershell
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -trimpath -ldflags '-s -w' -o w1f3.exe .
```

Run tests and static checks:

```powershell
go test ./...
go vet ./...
```

## Useful commands

```powershell
W1F3 -v
W1F3 --help
W1F3 --logo "Windows 11"
W1F3 --logo arch
W1F3 --speed 0.8
W1F3 --size 1.2 --depth 1.8
W1F3 --no-info
W1F3 --no-color
```

## Project layout

```text
W1F3/
├── main.go
├── main_test.go
├── platform_windows.go
├── platform_stub.go
├── config.yaml             # Canonical remote default; not embedded/bundled
├── config.schema.json      # Linked YAML editor schema
├── build.ps1
├── install.ps1
├── uninstall.ps1
├── w1f3.cmd
├── fetch.cmd               # Backward-compatible alias
├── go.mod
├── .gitignore
├── LICENSE
└── README.md
```

## GitHub repository setup

The intended repository URL is:

```text
https://github.com/sambui-Electron/W1F3
```

The application fetches its default config from the `main` branch, so keep `config.yaml` at the repository root and retain that branch name (or update `defaultConfigURL` in `main.go` if you change either).

## Acknowledgements

- Original concept: [areofyl/fetch](https://github.com/areofyl/fetch)
- System information backend: [Fastfetch](https://github.com/fastfetch-cli/fastfetch) ([module reference](https://github.com/fastfetch-cli/fastfetch/wiki/Modules))


### Disk usage display

The disk section lists each drive as `C (Windows): USED_GB/TOTAL_GB (PERCENT%)`. Values use unpadded one-decimal GiB formatting; the displayed unit is GB for readability.

### Adaptive scale limit

Window-size scaling uses the geometric mean of the terminal width and height relative to a 120×36 baseline. This allows wider terminals to contribute to scaling even when their aspect ratio is unusually wide. The adaptive multiplier never falls below 1.0x and is capped at 1.5x. If the configured or command-line base logo size is greater than 1.5x, the adaptive multiplier is disabled and the renderer uses the configured fixed logo-height limit, even when `adaptive_scale` is still set to `true`. A base size of exactly 1.5x still permits adaptive scaling. The logo can still contract to fit a terminal that is too small for its configured size. The rendered logo canvas is centered inside its allocated viewport on every frame, keeping fixed-height mode aligned as the terminal is resized; the mouse-drag hit area remains the full viewport. Disk entries are indented by four terminal columns beneath the `Disk:` heading.
