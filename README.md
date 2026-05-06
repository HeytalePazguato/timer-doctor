# timer-doctor

[![CI](https://img.shields.io/github/actions/workflow/status/HeytalePazguato/timer-doctor/ci.yml?branch=main&label=ci)](https://github.com/HeytalePazguato/timer-doctor/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/HeytalePazguato/timer-doctor)](go.mod)
[![Go Reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/HeytalePazguato/timer-doctor)
[![Latest release](https://img.shields.io/github/v/release/HeytalePazguato/timer-doctor?sort=semver)](https://github.com/HeytalePazguato/timer-doctor/releases)
[![License](https://img.shields.io/github/license/HeytalePazguato/timer-doctor)](LICENSE)

A CLI that audits systemd `.timer` units and prints a human-readable report.
Sibling to [cron-doctor](https://github.com/HeytalePazguato/cron-doctor).
Single static Go binary, no daemon, no D-Bus, no network.

## Install

**Homebrew (macOS, Linux):**

```sh
brew install HeytalePazguato/tap/timer-doctor
```

**Scoop (Windows):**

```pwsh
scoop bucket add timer-doctor https://github.com/HeytalePazguato/scoop-bucket
scoop install timer-doctor
```

**Go:**

```sh
go install github.com/HeytalePazguato/timer-doctor/cmd/timer-doctor@latest
```

**Pre-built binary (Linux, macOS):**

```sh
curl -sSL https://raw.githubusercontent.com/HeytalePazguato/timer-doctor/main/install.sh | sh
```

**Docker (GHCR, multi-arch):**

```sh
docker run --rm \
  -v /etc/systemd/system:/etc/systemd/system:ro \
  -v /usr/lib/systemd/system:/usr/lib/systemd/system:ro \
  ghcr.io/heytalepazguato/timer-doctor /etc/systemd/system
```

**Manual:** download the archive for your OS/arch from
[Releases](https://github.com/HeytalePazguato/timer-doctor/releases) — Linux,
macOS, and Windows on `amd64` and `arm64`.

**Build from source:**

```sh
git clone https://github.com/HeytalePazguato/timer-doctor.git
cd timer-doctor
go build -o timer-doctor ./cmd/timer-doctor
```

> **Why not npm / pip / cargo?** timer-doctor's audience is sysadmins
> managing systemd units, not Node/Python/Rust developers. The right
> channels for a sysadmin CLI are the OS-native package managers above.
> macOS and Windows are supported so you can lint a unit file before
> SCP'ing it to a Linux server.

## Usage

Audit a single timer file:

```sh
timer-doctor /etc/systemd/system/backup.timer
```

Audit every `.timer` file in a directory:

```sh
timer-doctor /etc/systemd/system
```

Audit a single OnCalendar expression (no file required):

```sh
timer-doctor "OnCalendar=*-*-* 04:00:00"
timer-doctor "*-*-* 04:00:00"
timer-doctor "Mon..Fri *-*-* 09:00"
```

Scan the system or user search paths:

```sh
timer-doctor --system           # /etc/systemd/system + /usr/lib/systemd/system
timer-doctor --user             # ~/.config/systemd/user + /usr/lib/systemd/user
```

Other modes:

```sh
timer-doctor --calendar /etc/systemd/system    # 7-day ASCII heatmap
timer-doctor --json /etc/systemd/system        # machine-readable output
timer-doctor --no-color /etc/systemd/system    # disable ANSI colors
```

### Sample output

#### Default text report

Run on `testdata/clean/backup.timer`. ANSI colors are added when stdout is a
TTY; `--no-color` produces the same body without escape codes.

```
backup.timer
  Service: backup.service
  Schedule (line 7): OnCalendar=*-*-* 04:00:00
    Every day at 04:00.
    Next: 2026-05-04 04:00, 2026-05-05 04:00, 2026-05-06 04:00.
  ⚠ WARN: Persistent= is not set. Runs missed during downtime will not be retried.
  ⚠ WARN: RandomizedDelaySec= unset on a popular calendar moment (04:00); risk of thundering herd.
  ✓ Service ExecStart /usr/local/bin/backup.sh exists and is executable.

1 timer audited — 0 errors, 2 warnings, 0 info.
```

#### Expression mode

Run on a bare OnCalendar string. Only schedule-related output is produced:
no service-pairing checks, no file lookups.

```
$ timer-doctor "OnCalendar=*-*-* 04:00:00"

Schedule: *-*-* 04:00:00
  Every day at 04:00.
  Next: 2026-05-04 04:00, 2026-05-05 04:00, 2026-05-06 04:00.
```

#### `--json`

Same fixture as the default report. Each timer gets its parsed schedules with
explanations, the next three fire times in RFC3339, the paired service path,
and an array of findings. A roll-up summary follows at the end.

```json
{
  "timers": [
    {
      "path": "/etc/systemd/system/backup.timer",
      "unit": "backup.timer",
      "service": "backup.service",
      "schedules": [
        {
          "type": "OnCalendar",
          "raw": "*-*-* 04:00:00",
          "explanation": "every day at 04:00",
          "next_runs": [
            "2026-05-04T04:00:00Z",
            "2026-05-05T04:00:00Z",
            "2026-05-06T04:00:00Z"
          ]
        }
      ],
      "findings": [
        {
          "severity": "warn",
          "code": "no_persistent",
          "message": "Persistent= is not set. Runs missed during downtime will not be retried."
        },
        {
          "severity": "warn",
          "code": "no_randomized_delay",
          "message": "RandomizedDelaySec= unset on a popular calendar moment (04:00); risk of thundering herd."
        }
      ]
    }
  ],
  "summary": {
    "timers": 1,
    "errors": 0,
    "warnings": 2,
    "info": 0
  }
}
```

#### `--calendar`

Run on a directory containing several timers. Hours along the top, days
down the side; each cell shows how many timer fires fall in that hour
(`.` = none). Hour-cells with two or more fires are listed below the grid
to surface unintentional thundering herds at midnight, on the hour, etc.

```
7-day calendar starting 2026-05-04

        0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15 16 17 18 19 20 21 22 23
Mon 04  2  .  .  .  1  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .
Tue 05  2  .  .  .  1  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .
Wed 06  2  .  .  .  1  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .
Thu 07  2  .  .  .  1  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .
Fri 08  2  .  .  .  1  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .
Sat 09  2  .  .  .  1  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .
Sun 10  2  .  .  .  1  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .  .

Collisions:
  Mon 04 00:00 — backup.timer, rotate.timer
```

## Checks

| Code                      | Severity | Triggers when                                                                                              |
|---------------------------|----------|------------------------------------------------------------------------------------------------------------|
| `parse_error`             | error    | Unit file is malformed or a schedule directive fails to parse.                                             |
| `missing_service`         | error    | Paired `.service` unit doesn't exist or can't be parsed.                                                   |
| `missing_exec_start`      | error    | Service's `ExecStart=` points at an absolute path that doesn't exist.                                      |
| `not_executable`          | warn     | `ExecStart=` target exists but lacks the execute bit.                                                      |
| `no_persistent`           | warn     | `OnCalendar=` set but `Persistent=true` is not — missed runs won't replay on next boot.                    |
| `no_randomized_delay`     | warn     | Timer fires at a popular calendar moment (`*:00:00`, `daily`, `hourly`, `weekly`) with no `RandomizedDelaySec=`. |
| `conflicting_pair`        | warn     | Two timers in the batch fire within 60 s of each other and write to similar paths.                         |
| `orphan_service`          | warn     | A `.service` in the same dir has `WantedBy=timers.target` (or no `[Install]`) but no matching `.timer`.    |
| `no_flock`                | warn     | Timer fires more often than every 10 minutes and the service has no `flock` / `pidof` / `Type=oneshot`.    |
| `wrong_accuracy`          | warn     | Timer fires every minute or more often with the default 1-minute `AccuracySec=`.                           |
| `bad_calendar`            | error    | `OnCalendar=` value fails calendar-syntax validation.                                                      |
| `no_hardening`            | info     | Service has none of `ProtectSystem=`, `PrivateTmp=`, `NoNewPrivileges=`. Hint, not a warning.              |

### Example finding

```
checkup.timer
  Service: checkup.service
  Schedule (line 5): OnCalendar=*:0/1
    Every minute.
    Next: 2026-05-04 04:23, 2026-05-04 04:24, 2026-05-04 04:25.
  ⚠ WARN: Timer fires more often than every 10 minutes; ExecStart has no flock/pidof/Type=oneshot. Concurrent runs may pile up.
  ⚠ WARN: Default AccuracySec=1min on a sub-minute timer can cause drift; set AccuracySec=1s explicitly.
  ⓘ INFO: Service has no hardening directives (ProtectSystem=, PrivateTmp=, NoNewPrivileges=).
```

## Project layout

```
cmd/timer-doctor/     CLI entry point and mode detection.
internal/parser/      .timer / .service unit-file parser (INI subset).
internal/audit/       English explainer + each lint check.
internal/calendar/    OnCalendar parser + systemd-analyze fallback.
internal/report/      Text, JSON, and calendar renderers.
testdata/             Fixtures used by unit tests and demos.
```

Each audit check is its own function in `internal/audit/checks.go`. Adding a
new check means adding one function and wiring it into `Run` in
`internal/audit/audit.go`.

The `internal/calendar` package wraps `systemd-analyze calendar` when it's
on `$PATH` and falls back to a built-in parser otherwise — this is what
lets timer-doctor run on macOS and Windows for offline linting.

## Out of scope (v0.1)

- Editing or fixing unit files (read-only tool).
- LLM features or any network calls.
- Web UI, daemon, watch mode.
- Timer creation or scheduling assistance.
- Localization (English only).
- Cross-machine timer aggregation.
- D-Bus integration (file parsing only — never talks to systemd at runtime).

## Branching & releases

```
develop  →  release/<version>  →  main
```

Never PR directly to `main`. `main` is reserved for stable releases only.

- **`develop`** — active development; daily integration target.
- **`release/<version>`** (e.g. `release/0.0.2`) — pre-release stabilization
  branch cut from `develop`.
- **`main`** — stable releases only; PRs come from `release/*`.

### Versioning source of truth

- Stable version lives in [`VERSION`](VERSION) and is bumped manually on the
  release branch before merging to `main`.
- Pre-release version is derived from the branch name
  (`release/0.0.2` → base `0.0.2`).
- Dev builds are stamped automatically as `0.0.<run_number>-dev`.
- The version is embedded into the binary at build time via
  `-ldflags="-X main.version=..."` and printed by `timer-doctor --version`.

### Changelog

User-visible changes are tracked in [`CHANGELOG.md`](CHANGELOG.md), following
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Add entries under
`[Unreleased]` while working on `develop` or `release/*`; on stable release,
rename that section to `[X.Y.Z] - YYYY-MM-DD` and start a fresh `[Unreleased]`.

### CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml))

Runs on push to `main`, `develop`, `release/**` and on PRs to `main`/`develop`.

| Trigger          | Job                                                  | Output                                                                                                              |
| ---------------- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Any branch above | `lint-build-test` (Go 1.22 + 1.23, Linux/macOS/Win)  | Gate for everything else.                                                                                           |
| Push `develop`   | `dev-build` ([`prerelease.yml`](.github/workflows/prerelease.yml)) | Snapshot binaries uploaded as artifact `timer-doctor-0.0.<run>-dev` (90d retention, latest 3 kept).      |
| Push `release/*` | `prerelease`  ([`prerelease.yml`](.github/workflows/prerelease.yml)) | Auto-tagged GitHub pre-release + binaries (latest 5 kept). Stage from commit msg: default `alpha`, `[beta]`, `[rc]`. |
| Push `main`      | `release` ([`release.yml`](.github/workflows/release.yml)) | Git tag `v<VERSION>` + GitHub Release + Homebrew tap update + Scoop bucket update.                                  |

### Pre-release stage selection

Stage is chosen by commit-message keyword on the release branch, with an
auto-incrementing counter per stage:

- default → alpha (e.g. `v0.0.2-alpha.1`, `v0.0.2-alpha.2`)
- `[beta]` in commit msg → beta (`v0.0.2-beta.1`, …)
- `[rc]` in commit msg → rc (`v0.0.2-rc.1`, …)

### Idempotence guard

The `release` job checks for an existing `v<VERSION>` tag and skips
tagging/publishing if it already exists — safe to re-run after a flake or
when `main` receives a non-version-bump commit.

### Required secrets

Repository → Settings → Secrets and variables → Actions:

| Secret                       | Used by                  | Why                                                                       |
| ---------------------------- | ------------------------ | ------------------------------------------------------------------------- |
| `GITHUB_TOKEN`               | all release jobs         | Auto-provided by Actions; tags & GitHub Release.                          |
| `HOMEBREW_TAP_GITHUB_TOKEN`  | release / prerelease     | PAT with `repo` scope on `HeytalePazguato/homebrew-tap`. Comment out the `brews:` block in `.goreleaser.yml` if you don't have a tap yet. |
| `SCOOP_GITHUB_TOKEN`         | release / prerelease     | PAT with `repo` scope on `HeytalePazguato/scoop-bucket`. Comment out the `scoops:` block in `.goreleaser.yml` if you don't have a bucket yet. |

## License

MIT — see [LICENSE](LICENSE).
