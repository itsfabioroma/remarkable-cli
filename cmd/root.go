package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var (
	flagTransport string
	flagJSON      bool
	flagHost      string
	flagPassword  string
	flagKeyPath   string
)

var rootCmd = &cobra.Command{
	Use:   "remarkable",
	Short: "CLI for reMarkable Paper Pro — SSH, Cloud, agent-native JSON",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
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

// getTransport connects using saved config or flags
func getTransport() (transport.Transport, error) {
	host := flagHost
	transportName := flagTransport

	// use saved config if no explicit flags
	if cfg := loadConfig(); cfg != nil {
		if !rootCmd.PersistentFlags().Changed("host") && cfg.Host != "" {
			host = cfg.Host
		}
		if !rootCmd.PersistentFlags().Changed("transport") && cfg.Transport != "" {
			transportName = cfg.Transport
		}
	}

	opts := sshOpts(host)

	switch transportName {
	case "ssh":
		t := transport.NewSSHTransport(opts...)
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
		return transport.AutoDetect(opts...)
	default:
		return nil, model.NewCLIError(model.ErrUnsupported, "",
			fmt.Sprintf("unknown transport: %s", transportName))
	}
}

// getSSH returns SSH transport specifically (for device management commands)
func getSSH() (*transport.SSHTransport, error) {
	host := flagHost
	if cfg := loadConfig(); cfg != nil {
		if !rootCmd.PersistentFlags().Changed("host") && cfg.Host != "" {
			host = cfg.Host
		}
	}

	t := transport.NewSSHTransport(sshOpts(host)...)
	if err := t.Connect(); err != nil {
		return nil, err
	}
	return t, nil
}

func sshOpts(host string) []transport.SSHOption {
	opts := []transport.SSHOption{transport.WithHost(host)}
	if flagPassword != "" {
		opts = append(opts, transport.WithPassword(flagPassword))
	}
	if flagKeyPath != "" {
		opts = append(opts, transport.WithKeyPath(flagKeyPath))
	}
	return opts
}

// --- output helpers ---

func output(data any) {
	if flagJSON || !isTerminal() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(data)
		return
	}

	// human output for doc lists
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
	if cliErr, ok := err.(*model.CLIError); ok && (flagJSON || !isTerminal()) {
		json.NewEncoder(os.Stderr).Encode(cliErr)
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

func isTerminal() bool {
	fi, _ := os.Stdout.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}
