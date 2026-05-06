---
layout: default
title: "timer-doctor"
description: "Audit your systemd timers"
---

# timer-doctor

A CLI that audits systemd `.timer` units and prints a human-readable
report. Sibling to [cron-doctor](https://github.com/HeytalePazguato/cron-doctor).
Single static Go binary, no daemon, no D-Bus, no network.

[View on GitHub](https://github.com/HeytalePazguato/timer-doctor){: .btn }
[Download latest release](https://github.com/HeytalePazguato/timer-doctor/releases/latest){: .btn }

## Install

```sh
brew install HeytalePazguato/tap/timer-doctor
go install github.com/HeytalePazguato/timer-doctor/cmd/timer-doctor@latest
```

See the [README](https://github.com/HeytalePazguato/timer-doctor#install) for
all install methods (Scoop, Docker/GHCR, install.sh, manual binaries).

## Links

- [Issues](https://github.com/HeytalePazguato/timer-doctor/issues)
- [Discussions](https://github.com/HeytalePazguato/timer-doctor/discussions)
- [Releases](https://github.com/HeytalePazguato/timer-doctor/releases)
- [Security policy](https://github.com/HeytalePazguato/timer-doctor/blob/main/SECURITY.md)
