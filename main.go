package main

import "github.com/itsfabioroma/remarkable-cli/cmd"

// version info injected at build time by goreleaser via -ldflags.
// Empty when built with a plain `go build` — in that case cmd/root.go
// falls back to runtime/debug.ReadBuildInfo() so both paths work.
var (
	version = ""
	commit  = ""
	date    = ""
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
