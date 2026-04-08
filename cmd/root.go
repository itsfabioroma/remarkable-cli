package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/itsfabioroma/remarkable-cli/pkg/transport"
	"github.com/itsfabioroma/remarkable-cli/pkg/update"
	"github.com/spf13/cobra"
)

var (
	flagTransport string
	flagJSON      bool
	flagHost      string
	flagPassword  string
	flagKeyPath   string
)

// version is the display string for --version. Populated at startup:
//  1. If main() set ldflag-injected version/commit/date (goreleaser), use those.
//  2. Otherwise fall back to runtime/debug.ReadBuildInfo (plain `go build`).
//  3. Otherwise "(dev)".
var version = "0.1.0 (dev)"

// SetVersionInfo is called from main.go with ldflag-injected values.
// When goreleaser builds a release, these are set. Plain `go build` leaves
// them empty and we fall back to runtime/debug.ReadBuildInfo.
func SetVersionInfo(v, c, d string) {
	version = formatVersion(v, c, d)
	rootCmd.Version = version
}

// formatVersion renders "v0.x.x (sha, date)" from either ldflag values or
// the embedded VCS info. Keeps a single canonical format for --version.
func formatVersion(ldVersion, ldCommit, ldDate string) string {
	ver := strings.TrimPrefix(ldVersion, "v")
	rev := ldCommit
	ts := ldDate

	// goreleaser's .CommitDate is RFC3339 — trim to yyyy-mm-dd
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		ts = t.Format("2006-01-02")
	}

	// fall back to runtime/debug when ldflags weren't set.
	// info.Main.Version is set when installed via `go install mod@vX.Y.Z`
	// (or @latest resolving to a tag) — this is how we pick up the right
	// version string for `go install`-based upgrades.
	if ver == "" || rev == "" || ts == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if ver == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
				ver = strings.TrimPrefix(info.Main.Version, "v")
			}
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					if rev == "" {
						rev = s.Value
					}
				case "vcs.time":
					if ts == "" {
						if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
							ts = t.Format("2006-01-02")
						}
					}
				}
			}
		}
	}

	if ver == "" {
		ver = "0.1.0"
	}
	if len(rev) >= 7 {
		rev = rev[:7]
	}
	if rev == "" {
		return ver + " (dev)"
	}
	if ts == "" {
		return fmt.Sprintf("%s (%s)", ver, rev)
	}
	return fmt.Sprintf("%s (%s, %s)", ver, rev, ts)
}

var rootCmd = &cobra.Command{
	Use:                        "remarkable",
	Short:                      "CLI for reMarkable Paper Pro — SSH, Cloud, agent-native JSON",
	Version:                    version,
	SilenceUsage:               true,
	SilenceErrors:              true,
	SuggestionsMinimumDistance: 2,
}

// errorEmitted tracks whether outputError already wrote — prevents double-emit
// when RunE bodies call outputError before returning the err
var errorEmitted bool

// Execute is the single funnel: every error becomes a CLIError before exit
func Execute() {
	// fire-and-forget GitHub check once per day (never blocks)
	update.BackgroundCheck()

	err := rootCmd.Execute()

	// show a one-line "update available" banner after the command succeeds
	// (stderr only, suppressed when --json or stderr is not a TTY)
	if err == nil {
		update.Notify(isStderrTerminal(), flagJSON)
		return
	}
	cliErr := toCLIError(err)
	if !errorEmitted {
		outputError(cliErr)
	}
	os.Exit(1)
}

// isStderrTerminal returns true when stderr is a real TTY (not a pipe/file).
// Used to gate the update banner so it never pollutes agent output.
func isStderrTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// toCLIError wraps any error into the structured envelope
func toCLIError(err error) *model.CLIError {
	var cliErr *model.CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}
	msg := err.Error()
	// cobra unknown-command / arg errors are plain errors
	low := strings.ToLower(msg)
	switch {
	case strings.HasPrefix(low, "unknown command"):
		return &model.CLIError{Message: msg, Code: model.ErrUnknownCommand}
	case strings.Contains(low, "accepts") && strings.Contains(low, "arg"),
		strings.Contains(low, "requires") && strings.Contains(low, "arg"),
		strings.HasPrefix(low, "unknown flag"),
		strings.HasPrefix(low, "invalid argument"),
		strings.HasPrefix(low, "flag needs"):
		return &model.CLIError{Message: msg, Code: model.ErrInvalidArgs}
	}
	return &model.CLIError{Message: msg, Code: model.ErrIO}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagTransport, "transport", "cloud", "transport: cloud (default), ssh, auto")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "JSON output (default for non-TTY)")
	rootCmd.PersistentFlags().StringVar(&flagHost, "host", "10.11.99.1", "device IP")
	rootCmd.PersistentFlags().StringVar(&flagPassword, "password", "", "SSH password")
	rootCmd.PersistentFlags().StringVar(&flagKeyPath, "key", "", "SSH key path")
}

// getTransport returns the configured transport.
// Default is cloud (works everywhere, no developer mode). Users who want SSH
// pass --transport=ssh; --transport=auto keeps the old cloud-first-with-SSH-
// fallback behavior for setups where cloud is flaky.
func getTransport() (transport.Transport, error) {
	cfg := loadConfig()

	// cloud and ssh are direct; only auto runs the fallback logic
	if flagTransport != "auto" {
		return connectExplicit(flagTransport)
	}

	// no saved config → tell the agent what to do
	if cfg == nil {
		return nil, verboseErr("no device configured",
			"remarkable connect <host>    # connect via SSH (USB or WiFi)",
			"remarkable connect --host <ip>  # then auth for cloud too",
			"remarkable auth              # set up cloud access")
	}

	// try cloud first (works for everyone, no developer mode needed)
	if cfg.HasCloud {
		t := transport.NewCloudTransport()
		if err := t.Connect(); err == nil {
			return t, nil
		}
		// cloud failed, try SSH fallback
		if cfg.HasSSH {
			host := cfg.Host
			if rootCmd.PersistentFlags().Changed("host") {
				host = flagHost
			}
			t := transport.NewSSHTransport(sshOpts(host)...)
			if err := t.Connect(); err == nil {
				return t, nil
			}
		}
		return nil, verboseErr("cloud unavailable and SSH failed",
			"remarkable connect    # reconnect")
	}

	// cloud not configured, try SSH
	if cfg.HasSSH {
		host := cfg.Host
		if rootCmd.PersistentFlags().Changed("host") {
			host = flagHost
		}
		t := transport.NewSSHTransport(sshOpts(host)...)
		if err := t.Connect(); err == nil {
			return t, nil
		}
		return nil, verboseErr("SSH unavailable",
			"remarkable connect    # reconnect")
	}

	return nil, verboseErr("no working transport in saved config",
		"remarkable connect    # reconnect")
}

// getSSH returns SSH specifically — for device management commands
// gives a clear error explaining WHY SSH is needed
func getSSH() (*transport.SSHTransport, error) {
	cfg := loadConfig()

	host := "10.11.99.1"
	if cfg != nil && cfg.Host != "" {
		host = cfg.Host
	}
	if rootCmd.PersistentFlags().Changed("host") {
		host = flagHost
	}

	t := transport.NewSSHTransport(sshOpts(host)...)
	if err := t.Connect(); err != nil {
		return nil, verboseErr("this command requires SSH (direct device access)",
			fmt.Sprintf("remarkable connect %s    # ensure SSH is available", host),
			"SSH is needed for: splash, password, setup-key, watch, export")
	}
	return t, nil
}

func connectExplicit(name string) (transport.Transport, error) {
	cfg := loadConfig()
	host := flagHost
	if cfg != nil && cfg.Host != "" && !rootCmd.PersistentFlags().Changed("host") {
		host = cfg.Host
	}

	switch name {
	case "ssh":
		t := transport.NewSSHTransport(sshOpts(host)...)
		if err := t.Connect(); err != nil {
			return nil, err
		}
		return t, nil
	case "cloud":
		t := transport.NewCloudTransport()
		if err := t.Connect(); err != nil {
			return nil, err
		}
		return t, nil
	case "auto":
		// auto is handled upstream by getTransport, never reaches here
		return nil, fmt.Errorf("internal: auto should not reach connectExplicit")
	default:
		return nil, verboseErr(fmt.Sprintf("unknown transport: %s", name),
			"valid transports: ssh, cloud")
	}
}

func sshOpts(host string) []transport.SSHOption {
	opts := []transport.SSHOption{transport.WithHost(host)}

	// flag password takes priority, then saved config password
	if flagPassword != "" {
		opts = append(opts, transport.WithPassword(flagPassword))
	} else if cfg := loadConfig(); cfg != nil && cfg.Password != "" {
		opts = append(opts, transport.WithPassword(cfg.Password))
	}

	if flagKeyPath != "" {
		opts = append(opts, transport.WithKeyPath(flagKeyPath))
	}
	return opts
}

// verboseErr creates an error with actionable hints for the agent
func verboseErr(msg string, hints ...string) error {
	full := msg
	for _, h := range hints {
		full += "\n  " + h
	}
	return &model.CLIError{Message: full, Code: model.ErrTransportUnavailable}
}

// --- output helpers ---

func output(data any) {
	if flagJSON || !isTerminal() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(data)
		return
	}

	if docs, ok := data.([]model.Document); ok {
		tree := model.NewTree(docs)
		printTree(tree, "", 0)
		return
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func outputError(err error) {
	errorEmitted = true
	if flagJSON || !isTerminal() {
		if cliErr, ok := err.(*model.CLIError); ok {
			json.NewEncoder(os.Stderr).Encode(cliErr)
			return
		}
		json.NewEncoder(os.Stderr).Encode(map[string]string{"error": err.Error()})
		return
	}
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
}

func printTree(tree *model.Tree, parentID string, depth int) {
	for _, doc := range tree.Children(parentID) {
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}

		extra := ""
		if doc.FileType != "" {
			extra = fmt.Sprintf(" [%s]", doc.FileType)
		}
		if doc.PageCount > 0 {
			extra += fmt.Sprintf(" (%d pages)", doc.PageCount)
		}

		if doc.IsFolder() {
			fmt.Printf("%s%s/%s\n", indent, doc.Name, extra)
			printTree(tree, doc.ID, depth+1)
		} else {
			fmt.Printf("%s%s%s\n", indent, doc.Name, extra)
		}
	}
}

// syncCloudDoc finalizes a cloud upload by building doc + root indexes
func syncCloudDoc(t transport.Transport, docID string) {
	if ct, ok := t.(*transport.CloudTransport); ok {
		if err := ct.SyncDoc(docID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cloud sync failed: %v\n", err)
		}
	}
}

func isTerminal() bool {
	fi, _ := os.Stdout.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}
