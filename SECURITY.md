# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in timer-doctor, please report it
responsibly.

**Preferred channel:** open a private security advisory through GitHub:
[Report a vulnerability](https://github.com/HeytalePazguato/timer-doctor/security/advisories/new).

**Do NOT** open a public GitHub issue for security vulnerabilities.

You can expect an initial response within 7 days. Confirmed issues will be
fixed in the next release; the advisory will be published with credit (unless
you prefer to remain anonymous).

## Scope

timer-doctor is a read-only auditor for systemd `.timer` and `.service`
unit files. It parses files on disk and never makes network calls or
talks to D-Bus. The realistic security surface is parsing untrusted unit
files (malformed input, path-traversal in `Unit=` references) and
running `systemd-analyze` as a subprocess when present.

In scope:

- Crashes, infinite loops, or denial-of-service from a crafted unit file.
- Path traversal or arbitrary file reads from a crafted `Unit=` value.
- Command injection via the `systemd-analyze` invocation.

Out of scope:

- Vulnerabilities in unit files themselves (timer-doctor reports them; it
  does not promise to find every misconfiguration).
- The behavior of `systemd-analyze` itself (upstream systemd).

## Supported Versions

Only the latest released minor version is supported with security updates.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| older   | :x:                |
