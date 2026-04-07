# Changelog

All notable changes to this project are documented here.
Format based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Release pipeline: goreleaser + GitHub Actions that build darwin/linux × amd64/arm64 binaries, archives, and checksums on every `v*` tag push.
- CI pipeline: pull-request workflow runs `go vet`, `gofmt` check, `go build`, and `go test ./...` on ubuntu + macos.
- `.github/` templates for bug reports, feature requests, PRs, plus Dependabot for `gomod` and `github-actions`.
- `remarkable update` now checks GitHub Releases first and falls back to commits/main when no release exists yet.
- `install.sh` downloads the latest release binary (with sha256 verification), falling back to a source build via `--source` or when no release is published.
- Self-update command with background version awareness — fire-and-forget GitHub check once per 24h, one-line banner on stderr when a newer commit is available, date-aware comparison so locally-ahead builds don't trigger bogus downgrades.
- Schema-aware cloud transport supporting sync index v3 + v4 with proper HashEntries and atomic root update protocol (1de08bf).
- e2e test suite under `e2e/` with read + write round-trip against a real cloud account, gated by `RMCLI_E2E_WRITE=1`.
- User-token caching in `pkg/auth/auth.go` (50min TTL) to avoid hammering the auth server across CLI invocations.
- Doc-index merge in `SyncDoc` so partial updates (mv, tag) preserve existing files instead of clobbering them.
- `Long` + `Example` on all 31 commands for copy-paste help output.
- Canonical JSON schema across document-returning commands (`ls`, `info`, `search`) using `model.Document` with `path` and `tags` as `omitempty` additions.
- New error codes `ErrIO`, `ErrInvalidArgs`, `ErrUnknownCommand`; cobra arg/unknown-command errors flow through the structured JSON envelope.
- "Did you mean?" suggestions for unknown commands via cobra `SuggestionsMinimumDistance`.
- `--version` shows injected version + commit hash + build date (ldflag or runtime/debug).
- Screenshot command and PDF source in export overlay (ba0f4cb, d9bd576).
- Cloud read/write via sync v3 blob protocol (1de08bf).
- Interactive `connect` wizard and cloud-first transport default (d4b3a32).
- `CONTRIBUTING.md`, `CHANGELOG.md`, `scripts/install.sh`.

### Changed

- Cloud is now the default transport; `--transport=auto` retained for legacy SSH-fallback behavior (d4b3a32).
- `mv` now properly syncs to cloud (previously a silent no-op).
- Cloud `ls` paced with persistent disk cache to prevent 429 rate limits (456820b, 73bc121).
- All file-I/O error paths across `cmd/` now produce a consistent `{"error","code"}` JSON envelope.

### Fixed

- Cloud `PUT` retries on 429 with `Retry-After`, plus delays between `SyncDoc` steps (fd1ec85).
- Global semaphore + content-addressed persistent blob cache to prevent rate-limiting under load.
- Cloud `PUT` protocol verified end-to-end (73bc121).
