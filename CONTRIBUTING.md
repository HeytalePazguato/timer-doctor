# Contributing to timer-doctor

Thanks for your interest! Contributions of any size are welcome — bug
reports, fixes, new features, doc improvements.

## Branch flow

```
develop  →  release/<version>  →  main
```

- All feature work, bug fixes, and refactors target `develop`.
- `main` is reserved for stable releases — never PR directly to it.
- Release stabilization happens on `release/<version>` branches cut from
  `develop`.

See the [README's "Branching & releases"](README.md) section for the full
versioning, tagging, and CI flow.

## Adding a new audit check

Each check is one function that takes a parsed timer (with paired service)
and returns `[]Finding`. To add one:

1. Drop a new function into `internal/audit/checks.go`.
2. Wire it into `Run` in `internal/audit/audit.go`.
3. Add a fixture under `testdata/` and a unit test that asserts the
   finding fires (and doesn't fire on a clean fixture).
4. Add a row to the checks table in `README.md`.

## Pull requests

- One logical change per PR; keep diffs reviewable.
- Run lint + build + test locally before opening the PR.
- Update `CHANGELOG.md` under `[Unreleased]`.
- Don't bump `VERSION` in feature PRs — that happens on the release branch.
- The `ci` workflow must pass before a PR can merge.

## Commit messages

Conventional-commits style is appreciated:

```
feat: add <thing>
fix: handle <edge case>
docs: clarify <section>
chore(deps): bump <dep> from X to Y
ci: <workflow change>
```

The release-branch workflow looks for `[beta]` / `[rc]` keywords in commit
messages to choose pre-release stages — keep those out of normal commits.

## Reporting bugs

Use the bug report template under "New Issue".
A minimal reproducing example is gold.

## Asking questions / proposing ideas

Use [Discussions](https://github.com/HeytalePazguato/timer-doctor/discussions)
for open-ended questions and ideas. Reserve issues for actionable bugs and
concrete feature requests.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
