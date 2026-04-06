package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/auth"
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
	Short: "Set up your reMarkable — cloud first, optional SSH for power users",
	Long: `Interactive setup wizard.

  remarkable connect                        # full wizard
  remarkable connect 192.168.1.5            # skip to SSH setup
  remarkable connect --cloud-only           # cloud only, no SSH prompt
  remarkable connect 192.168.1.5 --ssh-only # SSH only, no cloud prompt`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		if cfg == nil {
			cfg = &deviceConfig{}
		}

		reader := bufio.NewReader(os.Stdin)
		interactive := isTerminal()
		sshOnly, _ := cmd.Flags().GetBool("ssh-only")
		cloudOnly, _ := cmd.Flags().GetBool("cloud-only")

		// if host arg given, treat as SSH setup shortcut
		if len(args) > 0 {
			cfg.Host = args[0]
			if !cloudOnly {
				sshOnly = true
			}
		}

		// --- step 1: cloud ---
		if !sshOnly {
			cfg = setupCloud(cfg, reader, interactive)
		}

		// --- step 2: SSH (optional) ---
		if !cloudOnly {
			cfg = setupSSH(cfg, reader, interactive, args)
		}

		// must have at least one transport
		if !cfg.HasSSH && !cfg.HasCloud {
			fmt.Println("\nNo connection available.")
			return fmt.Errorf("no connection")
		}

		saveConfig(cfg)

		output(map[string]any{
			"hasCloud": cfg.HasCloud,
			"hasSSH":   cfg.HasSSH,
			"host":     cfg.Host,
		})

		if interactive {
			fmt.Println("\nSaved. You're ready to go!")
			if cfg.HasCloud && !cfg.HasSSH {
				fmt.Println("Tip: add SSH later for write access → remarkable connect <device-ip> --ssh-only")
			}
		}
		return nil
	},
}

// setupCloud handles cloud authentication
func setupCloud(cfg *deviceConfig, reader *bufio.Reader, interactive bool) *deviceConfig {
	store := auth.NewTokenStore()

	// check if already authed
	if _, err := auth.EnsureAuth(store); err == nil {
		cfg.HasCloud = true
		fmt.Println("  cloud: already connected ✓")
		return cfg
	}

	if interactive {
		fmt.Println("\n── Step 1: Cloud ──")
		fmt.Println("Go to https://my.remarkable.com/device/browser/connect")
		fmt.Println("Enter the 8-character code below (or press Enter to skip)")
		fmt.Println()
	}

	fmt.Print("Code: ")
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	// skip if empty
	if code == "" {
		fmt.Println("  cloud: skipped")
		return cfg
	}

	// register device
	tokens, err := auth.RegisterDevice(code)
	if err != nil {
		fmt.Printf("  cloud: failed (%s)\n", err)
		return cfg
	}

	if err := store.Save(tokens); err != nil {
		fmt.Printf("  cloud: registered but failed to save tokens (%s)\n", err)
		return cfg
	}

	cfg.HasCloud = true
	fmt.Println("  cloud: connected ✓")
	return cfg
}

// setupSSH handles SSH connection setup
func setupSSH(cfg *deviceConfig, reader *bufio.Reader, interactive bool, args []string) *deviceConfig {
	// if not already skipping and no host provided, ask
	if len(args) == 0 && cfg.Host == "" {
		if interactive && !cfg.HasSSH {
			fmt.Println("\n── Step 2: SSH (optional, for write access) ──")
			fmt.Println("Requires developer mode. Enter device IP or press Enter to skip.")
			fmt.Println()
			fmt.Print("Host: ")
			host, _ := reader.ReadString('\n')
			host = strings.TrimSpace(host)
			if host == "" {
				fmt.Println("  ssh: skipped")
				return cfg
			}
			cfg.Host = host
		} else if cfg.Host == "" {
			cfg.Host = "10.11.99.1"
		}
	}

	// ask for password if not already saved and not passed via flag
	if interactive && flagPassword == "" && cfg.Password == "" {
		fmt.Print("Password (Enter to skip): ")
		pass, _ := reader.ReadString('\n')
		pass = strings.TrimSpace(pass)
		if pass != "" {
			cfg.Password = pass
		}
	} else if flagPassword != "" {
		cfg.Password = flagPassword
	}

	// probe SSH
	opts := sshOpts(cfg.Host)
	ssh := transport.NewSSHTransport(opts...)
	if err := ssh.Connect(); err == nil {
		docs, err := ssh.ListDocuments()
		ssh.Close()
		if err == nil {
			cfg.HasSSH = true
			fmt.Printf("  ssh: connected (%d documents) ✓\n", len(docs))
		}
	} else {
		fmt.Printf("  ssh: unavailable (%s)\n", err)
	}

	return cfg
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
	connectCmd.Flags().Bool("ssh-only", false, "only set up SSH, skip cloud")
	connectCmd.Flags().Bool("cloud-only", false, "only set up cloud, skip SSH")
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(disconnectCmd)
}
