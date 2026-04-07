# Changelog

All notable changes to this project are documented here.
Format based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Schema-aware cloud transport supporting sync index v3 + v4 with proper HashEntries and atomic root update protocol (1de08bf).
- e2e test suite under `e2e/` with read + write round-trip against a real cloud account, gated by `RMCLI_E2E_WRITE=1`.
- User-token caching in `pkg/auth/auth.go` (50min TTL) to avoid hammering the auth server across CLI invocations.
- Doc-index merge in `SyncDoc` so partial updates (mv, tag) preserve existing files instead of clobbering them.
- Screenshot command and PDF source in export overlay (ba0f4cb, d9bd576).
- Cloud read/write via sync v3 blob protocol (1de08bf).
- Interactive `connect` wizard and cloud-first transport default (d4b3a32).

### Changed

- Cloud is now the default transport; `--transport=auto` retained for legacy SSH-fallback behavior (d4b3a32).
- `mv` now properly syncs to cloud (previously a silent no-op).
- Cloud `ls` paced with persistent disk cache to prevent 429 rate limits (456820b, 73bc121).

### Fixed

- Cloud `PUT` retries on 429 with `Retry-After`, plus delays between `SyncDoc` steps (fd1ec85).
- Global semaphore + content-addressed persistent blob cache to prevent rate-limiting under load.
- Cloud `PUT` protocol verified end-to-end (73bc121).
