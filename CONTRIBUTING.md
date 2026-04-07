# Contributing

Agent-focused notes for hacking on `remarkable-cli`.

## Build

```bash
go build -o remarkable .
```

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
