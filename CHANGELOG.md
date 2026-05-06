# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial release of `timer-doctor`: a CLI auditor for systemd `.timer`
  units, sibling to [cron-doctor](https://github.com/HeytalePazguato/cron-doctor).
- Input modes: single `.timer` file, directory batch, inline `OnCalendar=`
  expression, `--system` and `--user` search-path scans.
- Built-in `OnCalendar` parser with optional `systemd-analyze calendar`
  fallback when the binary is on `$PATH` (used for the next-fire-time
  computation).
- Twelve audit checks covering: parse errors, missing or non-executable
  service `ExecStart`, missing service unit, missing `Persistent=`,
  missing `RandomizedDelaySec=` on popular calendar moments, conflicting
  timer pairs (firing within 60 s and writing similar paths), orphan
  services, no-flock on high-frequency timers, default `AccuracySec=` on
  sub-minute timers, calendar-syntax errors, and a service-hardening hint.
- Output formats: ANSI-colored text (auto-detected TTY, `--no-color` to
  force plain), `--json`, and a 7-day ASCII heatmap via `--calendar`.
- `--version` flag stamped from `VERSION` at build time.
- GitHub Actions workflows: CI (lint + build + test on Linux, macOS,
  Windows), prerelease (`develop` snapshots and `release/*` tags), and
  stable release (`main` → `v<VERSION>` tag + GoReleaser publish).
- GoReleaser config for static binaries on linux-amd64, linux-arm64,
  darwin-amd64, darwin-arm64, windows-amd64, windows-arm64; Homebrew tap
  formula; Scoop bucket manifest; multi-arch GHCR image.
