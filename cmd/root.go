package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/itsfabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var (
	flagTransport string
	flagJSON      bool
	flagHost      string
	flagPassword  string
	flagKeyPath   string
)

// version is set at build time or defaults to dev
var version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:           "remarkable",
	Short:         "CLI for reMarkable Paper Pro — SSH, Cloud, agent-native JSON",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagTransport, "transport", "auto", "transport: ssh, cloud, auto")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "JSON output (default for non-TTY)")
	rootCmd.PersistentFlags().StringVar(&flagHost, "host", "10.11.99.1", "device IP")
	rootCmd.PersistentFlags().StringVar(&flagPassword, "password", "", "SSH password")
	rootCmd.PersistentFlags().StringVar(&flagKeyPath, "key", "", "SSH key path")
}

// getTransport returns the best available transport
// prefers SSH (faster, full access), falls back to cloud (listing, remote put)
func getTransport() (transport.Transport, error) {
	cfg := loadConfig()

	// explicit --transport flag overrides everything
	if rootCmd.PersistentFlags().Changed("transport") {
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
		return nil, fmt.Errorf("auto is the default, don't pass --transport auto")
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
