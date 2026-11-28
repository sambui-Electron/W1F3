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
- Configurable information catalog with 70 selectable fields: 66 queried from Fastfetch plus four Windows-native W1F3 fields (`disk`, `kernel`, `shell`, and `uptime`). The catalog follows Fastfetch 2.71.0 and omits six presentation/command modules that W1F3 handles itself or cannot configure (`logo`, `title`, `separator`, `break`, `command`, and `custom`).
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

### Selecting information fields

`fields` is an ordered list. The stock config enables a compact set of 15 fields and includes the remaining supported options as commented entries grouped by category. Uncomment a field to show it; comment or remove an active entry to hide it; reorder entries to change display order. The schema validates all 70 supported information fields and provides editor completion. Four entries (`disk`, `kernel`, `shell`, and `uptime`) are collected by W1F3 itself; the other 66 map to Fastfetch modules.

W1F3 requests only the selected Fastfetch modules, rather than querying a hard-coded subset. Windows-native implementations are used for `disk`, `kernel`, `shell`, and `uptime`. Other modules are collected from Fastfetch and formatted into the information panel. Fastfetch implements 69 of its 76 modules on Windows in the 2.71.0 source; an available module can still produce no line when the device, OS, drivers, permissions, or installed Fastfetch build do not provide a result. `publicip` and `weather` are optional network-backed modules; keep them commented unless you explicitly want those lookups.

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


## Structured YAML configuration

W1F3 keeps YAML as its configuration format. In addition to the original flat
`fields:` list, it supports an ordered `modules:` list for Fastfetch-style
layouts. Each module can define its own display key and key color.

Example:

```yaml
logo:
  type: file
  source: "./ascii.txt"
  color:
    "1": "#CDD6F4"
  padding:
    top: 1
    right: 3

display:
  separator: " "

renderer:
  speed: 0.45
  mouse: true

modules:
  - type: custom
    format: "Hardware"
  - type: cpu
    key: "│ ├ CPU "
    key_color: green
  - type: memory
    key: "└ └ Memory "
    key_color: green
  - type: break
  - type: uptime
    key: "› Uptime "
    key_color: magenta
  - type: colors
    padding_left: 18
    symbol: circle
```

Supported presentation modules are `break`, `custom`, and `colors`.
Information modules use the same module names exposed by W1F3's Fastfetch
field catalog. The Windows-native `os_age` module reports the age of the
current Windows installation. Legacy `fields:` configurations remain
supported.

The schema at `config.schema.json` provides YAML editor completion and
validation. Copy the structured example in `config.yaml` to
`%USERPROFILE%\\.config\\fetch\\config.yaml`, then adjust the logo path
for your machine.
