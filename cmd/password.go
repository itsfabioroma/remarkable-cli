package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

// xochitl config path on device
const xochitlConf = "/home/root/.config/remarkable/xochitl.conf"

var passwordCmd = &cobra.Command{
	Use:   "password [new-password]",
	Short: "Change the device SSH password",
	Long: `Change the SSH root password on the reMarkable.
Updates both /etc/shadow (Linux auth) and xochitl.conf (displayed in Settings).
Editing DeveloperPassword in xochitl.conf is the most reliable method:
xochitl syncs it to /etc/shadow on startup, and the file survives firmware updates.

Also sets up your SSH key so you don't need a password at all.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		sshT, ok := t.(*transport.SSHTransport)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"password requires SSH transport (use --transport ssh)")
			outputError(err)
			return err
		}

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

		if len(newPassword) == 0 {
			return fmt.Errorf("password cannot be empty")
		}

		// read current xochitl.conf
		confContent, err := sshT.RunCommand("cat " + xochitlConf)
		if err != nil {
			return model.NewCLIError(model.ErrNotFound, "ssh",
				"cannot read xochitl.conf")
		}

		// update DeveloperPassword in config
		lines := strings.Split(confContent, "\n")
		found := false
		for i, line := range lines {
			if strings.HasPrefix(line, "DeveloperPassword=") {
				lines[i] = "DeveloperPassword=" + newPassword
				found = true
				break
			}
		}

		if !found {
			// add under [General] section
			for i, line := range lines {
				if strings.TrimSpace(line) == "[General]" {
					lines = append(lines[:i+1], append([]string{"DeveloperPassword=" + newPassword}, lines[i+1:]...)...)
					found = true
					break
				}
			}
		}

		if !found {
			return model.NewCLIError(model.ErrCorruptedData, "ssh",
				"cannot find [General] section in xochitl.conf")
		}

		// write updated config back
		newConf := strings.Join(lines, "\n")
		escaped := strings.ReplaceAll(newConf, "'", "'\\''")
		_, err = sshT.RunCommand(fmt.Sprintf("printf '%%s' '%s' > %s", escaped, xochitlConf))
		if err != nil {
			return model.NewCLIError(model.ErrPermissionDenied, "ssh",
				"cannot write xochitl.conf")
		}

		// restart xochitl so it syncs password to /etc/shadow
		sshT.RunCommand("systemctl restart xochitl")

		output(map[string]any{
			"status":  "changed",
			"message": "Password updated. xochitl restarted to sync to /etc/shadow.",
		})

		if !flagJSON && isTerminal() {
			fmt.Println("Password changed. xochitl is restarting to apply.")
			fmt.Println("Tip: set up SSH keys for passwordless access:")
			fmt.Println("  ssh-copy-id root@10.11.99.1")
		}

		return nil
	},
}

// setupKeyCmd copies your SSH public key to the device
var setupKeyCmd = &cobra.Command{
	Use:   "setup-key",
	Short: "Copy SSH public key to device for passwordless access",
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		sshT, ok := t.(*transport.SSHTransport)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"setup-key requires SSH transport")
			outputError(err)
			return err
		}

		// find local public key
		home, _ := os.UserHomeDir()
		var pubKey []byte
		for _, name := range []string{"id_ed25519.pub", "id_rsa.pub"} {
			path := home + "/.ssh/" + name
			data, err := os.ReadFile(path)
			if err == nil {
				pubKey = data
				break
			}
		}

		if pubKey == nil {
			return fmt.Errorf("no SSH public key found at ~/.ssh/id_ed25519.pub or ~/.ssh/id_rsa.pub")
		}

		// ensure .ssh dir exists and write authorized_keys
		sshT.RunCommand("mkdir -p /home/root/.ssh && chmod 700 /home/root/.ssh")

		// append key (avoid duplicates)
		keyStr := strings.TrimSpace(string(pubKey))
		existing, _ := sshT.RunCommand("cat /home/root/.ssh/authorized_keys 2>/dev/null")
		if strings.Contains(existing, keyStr) {
			output(map[string]any{"status": "already_configured"})
			if !flagJSON && isTerminal() {
				fmt.Println("SSH key already installed on device.")
			}
			return nil
		}

		escaped := strings.ReplaceAll(keyStr, "'", "'\\''")
		_, err = sshT.RunCommand(fmt.Sprintf("echo '%s' >> /home/root/.ssh/authorized_keys && chmod 600 /home/root/.ssh/authorized_keys", escaped))
		if err != nil {
			return model.NewCLIError(model.ErrPermissionDenied, "ssh", "cannot write authorized_keys")
		}

		output(map[string]any{"status": "installed"})
		if !flagJSON && isTerminal() {
			fmt.Println("SSH key installed. You can now connect without a password.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(passwordCmd)
	rootCmd.AddCommand(setupKeyCmd)
}
