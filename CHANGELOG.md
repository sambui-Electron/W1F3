# Changelog

## v0.5.2 — Initial release

- Windows-native Go implementation of W1F3, powered by Fastfetch.
- Animated 3D logo with mouse-drag rotation and release momentum.
- Responsive side-by-side information panel and terminal-aware adaptive logo scaling.
- Fixed-height mode for user-selected logo scales above 1.5x, with per-frame viewport alignment.
- Native Windows uptime, shell, kernel label, and named-volume disk usage.
- Configurable information display with 70 selectable fields: 66 Fastfetch modules and four Windows-native W1F3 fields.
- YAML Language Server schema reference, synchronized JSON Schema completion/validation, and upgrade-time repair of existing config references.
- First-run configuration download, logo cache, installation and upgrade scripts, and legacy `fetch` command compatibility.

### Requirements

- Windows 10/11 x64.
- Fastfetch installed and available on `PATH`; version 2.68.0+ recommended for the complete module catalog.

### Configuration

The default configuration is retrieved from the repository on first launch and stored at `%USERPROFILE%\.config\fetch\config.yaml`. Existing configuration is preserved when upgrading.
