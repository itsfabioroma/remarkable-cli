package cmd

import (
	"github.com/fabioroma/remarkable-cli/pkg/daemon"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

// daemonCmd is launched by "connect" in the background — not user-facing
var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Start persistent connection daemon (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// build transport from flags
		opts := []transport.SSHOption{transport.WithHost(flagHost)}
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
			return err
		}

		// run daemon (blocks until stopped)
		return daemon.Start(t)
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}
