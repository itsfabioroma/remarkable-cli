# e2e tests

End-to-end tests that drive the built `remarkable` binary as a subprocess
against a real reMarkable Cloud account. No mocks.

## Run modes

```bash
# read-only — safe on any authenticated account
go test ./e2e -v

# full suite — creates and deletes docs named rmcli-e2e-*
RMCLI_E2E_WRITE=1 go test ./e2e -v
```

If `remarkable auth` hasn't been run, `TestMain` exits 0 and skips everything,
so `go test ./...` stays safe on fresh machines and CI boxes.

## Adding a test

- Read tests: `read_test.go` — assert against `RunJSON[T]`, no side effects.
- Write tests: `write_test.go` — gate with `SkipUnlessWrite(t)`, register
  `t.Cleanup(func() { cleanupDoc(t, name) })` BEFORE the assertions so a
  failure still cleans up. Use `UniqueName("prefix")` for stable,
  time-sortable names.
- Fixtures: `fixtures.go` — add tiny inline test files here (PDF, EPUB).

Shared helpers live in `harness.go`: `Build`, `Run`, `MustOK`, `RunJSON[T]`,
`SkipUnlessWrite`, `UniqueName`.
