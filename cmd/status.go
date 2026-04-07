package cmd

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/itsfabioroma/remarkable-cli/pkg/auth"
	"github.com/itsfabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Health check — SSH, cloud, firmware, storage, doc count",
	Long: `Probe the device: SSH reachability, cloud auth, firmware version, storage usage, and document count.`,
	Example: `  remarkable status
  remarkable --json status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()

		// resolve host
		host := "10.11.99.1"
		if cfg != nil && cfg.Host != "" {
			host = cfg.Host
		}
		if rootCmd.PersistentFlags().Changed("host") {
			host = flagHost
		}

		// probe SSH (TCP dial first, then full connect for details)
		sshAvailable := false
		firmwareVersion := ""
		storageInfo := ""
		docCount := -1

		conn, err := net.DialTimeout("tcp", host+":22", 2*time.Second)
		if err == nil {
			conn.Close()
			sshAvailable = true

			// get device details via SSH
			sshT := transport.NewSSHTransport(sshOpts(host)...)
			if err := sshT.Connect(); err == nil {
				defer sshT.Close()

				// firmware version
				if raw, err := sshT.RunCommand("cat /etc/os-release | grep IMG_VERSION"); err == nil {
					if parts := strings.SplitN(raw, "=", 2); len(parts) == 2 {
						firmwareVersion = strings.TrimSpace(parts[1])
					}
				}

				// storage
				if raw, err := sshT.RunCommand("df -h /home | tail -1"); err == nil {
					storageInfo = parseStorage(raw)
				}

				// document count
				if docs, err := sshT.ListDocuments(); err == nil {
					docCount = len(docs)
				}
			}
		}

		// probe cloud auth
		cloudAvailable := false
		store := auth.NewTokenStore()
		if _, err := auth.EnsureAuth(store); err == nil {
			cloudAvailable = true
		}

		// build result
		result := map[string]any{
			"host":      host,
			"ssh":       sshAvailable,
			"cloud":     cloudAvailable,
			"firmware":  firmwareVersion,
			"storage":   storageInfo,
			"documents": docCount,
		}

		// human-readable output for terminals
		if !flagJSON && isTerminal() {
			printStatus(result)
			return nil
		}

		output(result)
		return nil
	},
}

// printStatus renders human-readable status
func printStatus(r map[string]any) {
	fmt.Printf("Host:       %v\n", r["host"])

	if r["ssh"].(bool) {
		fmt.Printf("SSH:        connected\n")
	} else {
		fmt.Printf("SSH:        unavailable\n")
	}

	if r["cloud"].(bool) {
		fmt.Printf("Cloud:      connected\n")
	} else {
		fmt.Printf("Cloud:      unavailable\n")
	}

	if fw, _ := r["firmware"].(string); fw != "" {
		fmt.Printf("Firmware:   %s\n", fw)
	}

	if st, _ := r["storage"].(string); st != "" {
		fmt.Printf("Storage:    %s\n", st)
	}

	if dc, _ := r["documents"].(int); dc >= 0 {
		fmt.Printf("Documents:  %d\n", dc)
	}
}

// parseStorage extracts "used / total (pct)" from df output
func parseStorage(raw string) string {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) >= 5 {
		return fmt.Sprintf("%s / %s (%s)", fields[2], fields[1], fields[4])
	}
	return strings.TrimSpace(raw)
}

func init() { rootCmd.AddCommand(statusCmd) }
