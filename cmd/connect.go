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

// loadConfig reads saved device config
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

// saveConfig persists device config
func saveConfig(cfg *deviceConfig) error {
	dir := filepath.Dir(configPath())
	os.MkdirAll(dir, 0700)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(configPath(), data, 0600)
}

var connectCmd = &cobra.Command{
	Use:   "connect [host]",
	Short: "Connect to a reMarkable and remember it",
	Long: `Saves the device host and transport so future commands just work.
Without arguments, auto-detects via USB. With a host, connects via SSH over WiFi.

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

		// try connecting
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
		defer t.Close()

		// verify by listing docs
		docs, err := t.ListDocuments()
		if err != nil {
			outputError(err)
			return err
		}

		// save config
		cfg := &deviceConfig{
			Host:      host,
			Transport: t.Name(),
		}
		if flagTransport != "auto" {
			cfg.Transport = flagTransport
		}
		saveConfig(cfg)

		result := map[string]any{
			"status":    "connected",
			"host":      host,
			"transport": t.Name(),
			"documents": len(docs),
		}
		output(result)

		if !flagJSON && isTerminal() {
			fmt.Printf("Connected to reMarkable at %s via %s (%d documents)\n",
				host, t.Name(), len(docs))
			fmt.Println("Device saved. Future commands will use this connection automatically.")
		}

		return nil
	},
}

// disconnectCmd clears saved config
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
