# Changelog

## v0.0.2 - 2026-06-11

### Added
- Docker deployment support, including local image build/run targets and container documentation.
- Claude Code and Antigravity harness coverage.
- Native Python handler extensions with extension discovery and merge behavior.
- Source-event handler routing and inbound-event handler invocation deduplication.
- Typed Hitch event payload normalization.

### Fixed
- Handler invocation context propagation, shell wrapper payload arguments, reservation recording, and legacy handler compatibility.
- Native extension validation and override/shadowing edge cases.
- Configuration validation for unimplemented audit/observability backends.
- Installer release version derivation from tags.
- CI reliability for async observer dispatch test timing.

### Changed
- Split installer responsibilities across focused internal packages.
- Expanded handler invocation design and development documentation.
