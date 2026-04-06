package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var (
	watchOnChange string
)

var watchCmd = &cobra.Command{
	Use:   "watch [path]",
	Short: "Watch for document changes and exec hooks",
	Long:  "Polls the device for file changes. Use --on-change to exec a command on each change.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// must be DeviceTransport
		dt, ok := t.(transport.DeviceTransport)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"watch requires SSH transport (use --transport ssh)")
			outputError(err)
			return err
		}

		// setup signal handling for clean shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		// start watching
		changes, err := dt.WatchChanges(ctx)
		if err != nil {
			outputError(err)
			return err
		}

		if !flagJSON && isTerminal() {
			fmt.Println("Watching for changes... (Ctrl+C to stop)")
		}

		for event := range changes {
			// output the event
			output(map[string]any{
				"docId": event.DocID,
				"type":  event.Type,
				"path":  event.Path,
			})

			// exec hook if configured
			if watchOnChange != "" {
				hookCmd := strings.ReplaceAll(watchOnChange, "{id}", event.DocID)
				hookCmd = strings.ReplaceAll(hookCmd, "{type}", event.Type)

				c := exec.CommandContext(ctx, "sh", "-c", hookCmd)
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				c.Run() // don't fail watch on hook errors
			}
		}

		return nil
	},
}

func init() {
	watchCmd.Flags().StringVar(&watchOnChange, "on-change", "",
		"command to exec on change. Supports {id} and {type} placeholders")
	rootCmd.AddCommand(watchCmd)
}
