# W1F3

**W1F3** (pronounced “Wife”) is a Windows-native, animated 3D system-information viewer inspired by [areofyl/fetch](https://github.com/areofyl/fetch) and powered by [Fastfetch](https://github.com/fastfetch-cli/fastfetch).

W1F3 is written in Go and targets Windows 10/11 x64.

## Features

- Animated 3D Fastfetch logo with mouse drag rotation and release momentum.
- Live system-information panel.
- Windows uptime, shell, terminal, CPU, GPU, memory and named-volume information.
- Responsive layout and adaptive logo scaling.
- User configuration at `%USERPROFILE%\.config\fetch\config.yaml`.
- First-run default configuration downloaded from this repository's `main/config.yaml` on GitHub.
- JSON Schema for YAML editor validation and completion.
- Cached logos stored under `%USERPROFILE%\.config\fetch\logos\`.
- `fetch -v` / `fetch --version` reports the installed version.

## Requirements

- Windows 10 or 11, x64.
- Fastfetch installed and available as `fastfetch.exe` on `PATH`.
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
   fetch -v
   fetch
   ```

The installer upgrades the existing installation in `%LOCALAPPDATA%\Programs\Fetch`, preserves the user's configuration and logo cache, installs the schema, and updates the user PATH. It does **not** install or overwrite `config.yaml`.

## First launch and remote default configuration

When W1F3 cannot find `%USERPROFILE%\.config\fetch\config.yaml`, it downloads the canonical default from:

```text
https://raw.githubusercontent.com/sambui-Electron/W1F3/main/config.yaml
```

The downloaded YAML is validated by W1F3's parser before being written locally. Existing user configuration is never replaced or silently refreshed. If GitHub cannot be reached on first launch, W1F3 reports the download error and leaves the configuration absent so the user can retry or create it manually.

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

The repository's `config.yaml` contains the default configuration. The installer copies `config.schema.json` into the user config directory when it is not already present.

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
go build -trimpath -ldflags '-s -w' -o fetch.exe .
```

Run tests and static checks:

```powershell
go test ./...
go vet ./...
```

## Useful commands

```powershell
fetch -v
fetch --help
fetch --logo "Windows 11"
fetch --logo arch
fetch --speed 0.8
fetch --size 1.2 --depth 1.8
fetch --no-info
fetch --no-color
```

## Project layout

```text
W1F3/
├── main.go
├── main_test.go
├── platform_windows.go
├── platform_stub.go
├── config.yaml             # Canonical remote default; not embedded/bundled
├── config.schema.json
├── build.ps1
├── install.ps1
├── uninstall.ps1
├── fetch.cmd
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
- System information backend: [Fastfetch](https://github.com/fastfetch-cli/fastfetch)
