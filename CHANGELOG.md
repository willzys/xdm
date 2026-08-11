# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.1.0] - 2026-08-11

### Added

- Keyboard-first terminal client for X Direct Messages.
- Local demo backend for interface development without credentials.
- Browser-based authentication for the X web backend.
- Experimental encrypted XChat messaging support.
- Local account-data removal commands.
- Self-contained XChat crypto helper for packaged builds.
- Release archives for Windows x64, Linux x64, macOS Intel, and macOS Apple Silicon.
- SHA-256 checksums for published release archives.

### Changed

- Improved composer and conversation-search keyboard navigation.
- Added project contribution, security, support, and governance documentation.

### Fixed

- Prevented web login from attaching to unrelated remote-debugging browser sessions.

### Security

- Removed `NODE_OPTIONS` and `NODE_PATH` from the XChat crypto subprocess environment.

[Unreleased]: https://github.com/willzys/xdm/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/willzys/xdm/releases/tag/v0.1.0
