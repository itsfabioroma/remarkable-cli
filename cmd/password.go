package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

const xochitlConf = "/home/root/.config/remarkable/xochitl.conf"

var passwordCmd = &cobra.Command{
	Use:   "password [new-password]",
	Short: "Change the device SSH password",
	Long:  "Updates DeveloperPassword in xochitl.conf (survives firmware updates), restarts xochitl.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		// get new password
		var newPassword string
		if len(args) > 0 {
			newPassword = args[0]
		} else {
			fmt.Print("New password: ")
			reader := bufio.NewReader(os.Stdin)
			newPassword, _ = reader.ReadString('\n')
			newPassword = strings.TrimSpace(newPassword)
		}

		if newPassword == "" {
			return fmt.Errorf("password cannot be empty")
		}

		// read + update xochitl.conf
		conf, err := sshT.RunCommand("cat " + xochitlConf)
		if err != nil {
			return model.NewCLIError(model.ErrNotFound, "ssh", "cannot read xochitl.conf")
		}

		lines := strings.Split(conf, "\n")
		found := false
		for i, line := range lines {
			if strings.HasPrefix(line, "DeveloperPassword=") {
				lines[i] = "DeveloperPassword=" + newPassword
				found = true
				break
			}
		}
		if !found {
			for i, line := range lines {
				if strings.TrimSpace(line) == "[General]" {
					lines = append(lines[:i+1], append([]string{"DeveloperPassword=" + newPassword}, lines[i+1:]...)...)
					break
				}
			}
		}

		escaped := strings.ReplaceAll(strings.Join(lines, "\n"), "'", "'\\''")
		sshT.RunCommand(fmt.Sprintf("printf '%%s' '%s' > %s", escaped, xochitlConf))
		sshT.RunCommand("systemctl restart xochitl")

		output(map[string]any{"status": "changed"})
		return nil
	},
}

var setupKeyCmd = &cobra.Command{
	Use:   "setup-key",
	Short: "Install SSH public key for passwordless access",
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		home, _ := os.UserHomeDir()
		var pubKey []byte
		for _, name := range []string{"id_ed25519.pub", "id_rsa.pub"} {
			if data, err := os.ReadFile(home + "/.ssh/" + name); err == nil {
				pubKey = data
				break
			}
		}
		if pubKey == nil {
			return fmt.Errorf("no SSH public key found at ~/.ssh/")
		}

		keyStr := strings.TrimSpace(string(pubKey))
		existing, _ := sshT.RunCommand("cat /home/root/.ssh/authorized_keys 2>/dev/null")
		if strings.Contains(existing, keyStr) {
			output(map[string]any{"status": "already_installed"})
			return nil
		}

		sshT.RunCommand("mkdir -p /home/root/.ssh && chmod 700 /home/root/.ssh")
		escaped := strings.ReplaceAll(keyStr, "'", "'\\''")
		sshT.RunCommand(fmt.Sprintf("echo '%s' >> /home/root/.ssh/authorized_keys && chmod 600 /home/root/.ssh/authorized_keys", escaped))

		output(map[string]any{"status": "installed"})
		return nil
	},
}

func init() {
	rootCmd.AddCommand(passwordCmd)
	rootCmd.AddCommand(setupKeyCmd)
}
