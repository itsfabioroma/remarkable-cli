# Contributing

Agent-focused notes for hacking on `remarkable-cli`. PRs welcome.

## Quick start

```bash
git clone https://github.com/itsfabioroma/remarkable-cli
cd remarkable-cli
go build -o remarkable .
go test ./...
```

## Pull request flow

1. Fork the repo + create a branch off `main`.
2. Make the change. Keep commits small, use conventional-commit prefixes
   (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`) — the release changelog
   groups them automatically.
3. Run `go test ./...` and `gofmt -w .` locally. CI runs `go vet`, `gofmt`
   check, and the test matrix on ubuntu + macos.
4. Open a PR against `main`. The PR template has a short checklist.
5. For cloud-affecting changes, run the e2e suite locally first (needs a
   real account):
   ```bash
   RMCLI_E2E_WRITE=1 go test ./e2e -v
   ```

## Releasing

Releases are cut from git tags via goreleaser.

```bash
git tag -a v0.2.0 -m "release v0.2.0"
git push origin v0.2.0
```

`.github/workflows/release.yml` picks up the tag and publishes a GitHub
Release with darwin/linux × amd64/arm64 binaries, archives, and a
`checksums.txt`. Users then get the update via `remarkable update`.

## Test

```bash
# unit + integration
go test ./...

# e2e against real cloud account (writes data)
RMCLI_E2E_WRITE=1 go test ./e2e -v
```

## Layout

- `cmd/*.go` — one file per cobra command (`ls.go`, `put.go`, `mv.go`, ...).
- `pkg/transport/cloud.go` — cloud sync v3/v4 transport (default).
- `pkg/transport/ssh.go` — SSH transport (fast, full access).
- `pkg/auth/` — cloud device registration + token caching.
- `pkg/model/` — `Document`, `Metadata`, `CLIError` envelope.
- `pkg/encoding/`, `pkg/render/`, `pkg/extract/` — `.rm` v6 parser, SVG/PNG render, EPUB/highlight extract.

## Adding a command

1. Copy an existing `cmd/*.go` (e.g. `cmd/ls.go`) as a template.
2. Define a `cobra.Command`, set `Use`/`Short`/`Args`/`RunE`.
3. Register in `init()` with `rootCmd.AddCommand(myCmd)`.
4. Acquire transport via `getTransport()`; always `defer t.Close()`.

## Conventions

- JSON output: call `output(v)` — never `fmt.Println` raw structs.
- Errors: build with `model.NewCLIError(code, path, msg)` and emit via `outputError(err)` before returning.
- After any cloud write (`put`, `mv`, `rm`, `mkdir`, `tags`, `pages`), call `syncCloudDoc(t, docID)` so the doc-index merges instead of clobbering.
- Keep cmd files small + modular; push logic into `pkg/`.
- Blank lines between blocks; concise comment on top of each block.

## Don't

- Don't bypass `output()` / `outputError()` — agents parse stdout as JSON.
- Don't add new top-level packages without a reason; prefer extending `pkg/transport` or `pkg/model`.
- Don't commit credentials or `~/.config/remarkable-cli/` fixtures.
