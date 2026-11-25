# Contributing to PinchTab

This repository keeps one detailed contributor guide:

- [docs/guides/contributing.md](docs/guides/contributing.md)

Use that guide for setup, build commands, tests, hooks, and the day-to-day development workflow.

## Required formatting for contributors and coding agents

After editing Go files, run `gofmt -w` on the exact files changed. Before each
commit and push, run `gofmt -l` on those files and require empty output, then
run `git diff --check`. Repeat these checks after the final edit.

Use the Go version declared in `go.mod`. CI checks formatting across the
repository; do not disable that check. Report pre-existing formatting failures
outside the authorized scope instead of silently reformatting unrelated files.
Keep formatting-only fixes separate from behavior changes. Do not change IDE
settings or install hooks implicitly.

## Quick Start

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
./dev doctor
./dev check
./dev test unit
```

## Pull Requests

Please keep GitHub's "Allow edits from maintainers" option enabled on your PRs. It makes small fixes, rebases, and merge-conflict resolution much faster.
