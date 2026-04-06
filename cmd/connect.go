package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

// config persisted between invocations
type deviceConfig struct {
	Host      string `json:"host"`
	Transport string `json:"transport"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "remarkable-cli", "device.json")
}

func loadConfig() *deviceConfig {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil
	}
	var cfg deviceConfig
	json.Unmarshal(data, &cfg)
	if cfg.Host == "" {
		return nil
	}
	return &cfg
}

func saveConfig(cfg *deviceConfig) error {
	dir := filepath.Dir(configPath())
	os.MkdirAll(dir, 0700)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(configPath(), data, 0600)
}

var connectCmd = &cobra.Command{
	Use:   "connect [host]",
	Short: "Connect to a reMarkable and remember it",
	Long: `Verifies the device is reachable and saves the connection for future commands.

Examples:
  remarkable connect                    # auto-detect via USB
  remarkable connect 172.20.10.2        # WiFi IP
  remarkable connect --transport cloud  # cloud only`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := flagHost
		if len(args) > 0 {
			host = args[0]
		}

		// verify connectivity
		opts := []transport.SSHOption{transport.WithHost(host)}
		if flagPassword != "" {
			opts = append(opts, transport.WithPassword(flagPassword))
		}
		if flagKeyPath != "" {
			opts = append(opts, transport.WithKeyPath(flagKeyPath))
		}

		var t transport.Transport
		var err error

		switch flagTransport {
		case "cloud":
			ct := transport.NewCloudTransport()
			err = ct.Connect()
			t = ct
		case "ssh":
			st := transport.NewSSHTransport(opts...)
			err = st.Connect()
			t = st
		default:
			t, err = transport.AutoDetect(opts...)
		}

		if err != nil {
			outputError(err)
			return err
		}

		docs, err := t.ListDocuments()
		t.Close()
		if err != nil {
			outputError(err)
			return err
		}

		// save config
		transportName := t.Name()
		if flagTransport != "auto" {
			transportName = flagTransport
		}
		saveConfig(&deviceConfig{Host: host, Transport: transportName})

		output(map[string]any{
			"status":    "connected",
			"host":      host,
			"transport": transportName,
			"documents": len(docs),
		})

		if !flagJSON && isTerminal() {
			fmt.Printf("Connected to reMarkable at %s via %s (%d documents)\n",
				host, transportName, len(docs))
		}

		return nil
	},
}

var disconnectCmd = &cobra.Command{
	Use:   "disconnect",
	Short: "Forget the saved device connection",
	RunE: func(cmd *cobra.Command, args []string) error {
		os.Remove(configPath())
		output(map[string]any{"status": "disconnected"})
		if !flagJSON && isTerminal() {
			fmt.Println("Device config cleared.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(disconnectCmd)
}
