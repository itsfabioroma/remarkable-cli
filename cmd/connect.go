package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/daemon"
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
	Short: "Connect to a reMarkable and keep the connection alive",
	Long: `Connects to the device, saves config, and starts a background daemon
that keeps SSH open. Future commands talk to the daemon — instant response.

Examples:
  remarkable connect                    # auto-detect via USB
  remarkable connect 172.20.10.2        # WiFi IP
  remarkable connect --transport cloud  # cloud only`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// stop existing daemon if running
		if daemon.IsRunning() {
			daemon.Stop()
			time.Sleep(200 * time.Millisecond)
		}

		host := flagHost
		if len(args) > 0 {
			host = args[0]
		}

		// try connecting to verify
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

		// verify
		docs, err := t.ListDocuments()
		if err != nil {
			t.Close()
			outputError(err)
			return err
		}
		t.Close()

		// save config
		transportName := t.Name()
		if flagTransport != "auto" {
			transportName = flagTransport
		}
		saveConfig(&deviceConfig{Host: host, Transport: transportName})

		// start daemon in background
		exe, _ := os.Executable()
		daemonCmd := exec.Command(exe, "daemon",
			"--host", host,
			"--transport", transportName,
		)
		if flagPassword != "" {
			daemonCmd.Args = append(daemonCmd.Args, "--password", flagPassword)
		}
		if flagKeyPath != "" {
			daemonCmd.Args = append(daemonCmd.Args, "--key", flagKeyPath)
		}

		// detach from terminal
		daemonCmd.Stdout = nil
		daemonCmd.Stderr = nil
		daemonCmd.Stdin = nil
		if err := daemonCmd.Start(); err != nil {
			return fmt.Errorf("cannot start daemon: %w", err)
		}
		daemonCmd.Process.Release()

		// wait for daemon to be ready and cache warmed
		for i := 0; i < 50; i++ {
			time.Sleep(100 * time.Millisecond)
			if daemon.IsRunning() {
				// verify cache is populated
				resp, err := daemon.SendCommand(daemon.Request{Command: "ping"})
				if err == nil && resp.OK {
					break
				}
			}
		}

		result := map[string]any{
			"status":    "connected",
			"host":      host,
			"transport": transportName,
			"documents": len(docs),
			"daemon":    daemon.IsRunning(),
		}
		output(result)

		if !flagJSON && isTerminal() {
			fmt.Printf("Connected to reMarkable at %s via %s (%d documents)\n",
				host, transportName, len(docs))
			if daemon.IsRunning() {
				fmt.Println("Background daemon started. Commands will be instant.")
			}
		}

		return nil
	},
}

var disconnectCmd = &cobra.Command{
	Use:   "disconnect",
	Short: "Disconnect and stop the background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		daemon.Stop()
		os.Remove(configPath())

		output(map[string]any{"status": "disconnected"})
		if !flagJSON && isTerminal() {
			fmt.Println("Disconnected. Daemon stopped.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(disconnectCmd)
}
