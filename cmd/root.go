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
	// global flags
	flagTransport string
	flagJSON      bool
	flagHost      string
	flagPassword  string
	flagKeyPath   string
)

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "remarkable",
	Short: "Swiss army knife for reMarkable tablets",
	Long:  "A self-contained CLI to interact with reMarkable Paper Pro. SSH, USB, and Cloud.",
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagTransport, "transport", "auto", "transport: ssh, usb, cloud, auto")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "output as JSON (default for non-TTY)")
	rootCmd.PersistentFlags().StringVar(&flagHost, "host", "10.11.99.1", "device IP address")
	rootCmd.PersistentFlags().StringVar(&flagPassword, "password", "", "SSH password")
	rootCmd.PersistentFlags().StringVar(&flagKeyPath, "key", "", "SSH private key path")
}

// getTransport connects to the device using saved config, flags, or auto-detect
func getTransport() (transport.Transport, error) {
	// merge saved config with CLI flags (flags override saved config)
	host := flagHost
	transportName := flagTransport

	if cfg := loadConfig(); cfg != nil {
		// use saved host if user didn't explicitly set --host
		if !rootCmd.PersistentFlags().Changed("host") && cfg.Host != "" {
			host = cfg.Host
		}
		// use saved transport if user didn't explicitly set --transport
		if !rootCmd.PersistentFlags().Changed("transport") && cfg.Transport != "" {
			transportName = cfg.Transport
		}
	}

	opts := []transport.SSHOption{
		transport.WithHost(host),
	}
	if flagPassword != "" {
		opts = append(opts, transport.WithPassword(flagPassword))
	}
	if flagKeyPath != "" {
		opts = append(opts, transport.WithKeyPath(flagKeyPath))
	}

	switch transportName {
	case "ssh":
		t := transport.NewSSHTransport(opts...)
		if err := t.Connect(); err != nil {
			return nil, err
		}
		return t, nil

	case "usb":
		t := transport.NewUSBTransport()
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

// output prints data as JSON or human-readable
func output(data any) {
	if flagJSON || !isTerminal() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(data)
		return
	}

	// human output depends on the data type
	switch v := data.(type) {
	case []model.Document:
		printDocList(v)
	default:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(data)
	}
}

// outputError prints an error as JSON or plain text
func outputError(err error) {
	if cliErr, ok := err.(*model.CLIError); ok {
		if flagJSON || !isTerminal() {
			json.NewEncoder(os.Stderr).Encode(cliErr)
			return
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", cliErr.Error())
		return
	}

	if flagJSON || !isTerminal() {
		json.NewEncoder(os.Stderr).Encode(model.CLIError{
			Message: err.Error(),
			Code:    "unknown",
		})
		return
	}
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
}

// printDocList prints documents as a human-readable table
func printDocList(docs []model.Document) {
	tree := model.NewTree(docs)
	printTree(tree, "", 0)
}

func printTree(tree *model.Tree, parentID string, depth int) {
	for _, doc := range tree.Children(parentID) {
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}

		icon := "📄"
		if doc.IsFolder() {
			icon = "📁"
		}

		extra := ""
		if doc.FileType != "" {
			extra = fmt.Sprintf(" [%s]", doc.FileType)
		}
		if doc.PageCount > 0 {
			extra += fmt.Sprintf(" (%d pages)", doc.PageCount)
		}

		fmt.Printf("%s%s %s%s\n", indent, icon, doc.Name, extra)

		// recurse into folders
		if doc.IsFolder() {
			printTree(tree, doc.ID, depth+1)
		}
	}
}

// isTerminal checks if stdout is a terminal
func isTerminal() bool {
	fi, _ := os.Stdout.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}
