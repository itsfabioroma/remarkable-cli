# Agent instructions

Go CLI for reMarkable Paper Pro. Build with `go build -o remarkable .`, test with `go test ./...`.

The skill at `.agents/skills/remarkable-cli/SKILL.md` documents every command.

## Key facts

- Single binary: `./remarkable`
- JSON output by default when stdout is not a terminal
- SSH is the primary transport (~1s), cloud is fallback (~20s)
- Run `./remarkable connect <host>` before any other command
- Real .rm v6 test fixtures in `testdata/fixtures/`
- Errors include actionable hints — read them instead of guessing
