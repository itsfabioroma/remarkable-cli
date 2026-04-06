package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/itsfabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

// deviceConfig stores what's available after connect
type deviceConfig struct {
	Host     string `json:"host,omitempty"`
	Password string `json:"password,omitempty"`
	HasSSH   bool   `json:"hasSSH"`
	HasCloud bool   `json:"hasCloud"`
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
	Short: "Connect to a reMarkable — probes SSH + cloud, saves both",
	Long: `Probes SSH and cloud connectivity, saves what's available.
Future commands auto-pick the best transport per operation.

  remarkable connect              # USB default (10.11.99.1)
  remarkable connect 192.168.1.5  # WiFi IP`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := flagHost
		if len(args) > 0 {
			host = args[0]
		}

		cfg := &deviceConfig{Host: host}
		opts := sshOpts(host)

		// probe SSH
		ssh := transport.NewSSHTransport(opts...)
		if err := ssh.Connect(); err == nil {
			docs, err := ssh.ListDocuments()
			ssh.Close()
			if err == nil {
				cfg.HasSSH = true
				if flagPassword != "" {
					cfg.Password = flagPassword
				}
				fmt.Printf("  ssh: connected (%d documents)\n", len(docs))
			}
		} else {
			fmt.Printf("  ssh: unavailable (%s)\n", err)
		}

		// probe cloud
		cloud := transport.NewCloudTransport()
		if err := cloud.Connect(); err == nil {
			docs, err := cloud.ListDocuments()
			cloud.Close()
			if err == nil {
				cfg.HasCloud = true
				fmt.Printf("  cloud: connected (%d documents)\n", len(docs))
			}
		} else {
			fmt.Printf("  cloud: unavailable (run 'remarkable auth' to set up)\n")
		}

		if !cfg.HasSSH && !cfg.HasCloud {
			fmt.Println("\nNo connection available.")
			fmt.Println("  SSH: plug in USB or connect to same WiFi, then retry")
			fmt.Println("  Cloud: run 'remarkable auth' first")
			return fmt.Errorf("no connection")
		}

		saveConfig(cfg)

		output(map[string]any{
			"host":     host,
			"hasSSH":   cfg.HasSSH,
			"hasCloud": cfg.HasCloud,
		})

		if isTerminal() {
			fmt.Println("\nSaved. Commands will auto-pick the best transport.")
		}
		return nil
	},
}

var disconnectCmd = &cobra.Command{
	Use:   "disconnect",
	Short: "Forget saved connection",
	RunE: func(cmd *cobra.Command, args []string) error {
		os.Remove(configPath())
		fmt.Println("Disconnected.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(disconnectCmd)
}
