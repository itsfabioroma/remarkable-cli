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

var watchOnChange string

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch for document changes and exec hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		w, ok := t.(transport.Watchable)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(), "watch requires SSH")
			outputError(err)
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() { <-sigCh; cancel() }()

		changes, err := w.WatchChanges(ctx)
		if err != nil {
			outputError(err)
			return err
		}

		if !flagJSON && isTerminal() {
			fmt.Println("Watching... (Ctrl+C to stop)")
		}

		for event := range changes {
			output(map[string]any{"docId": event.DocID, "type": event.Type})

			if watchOnChange != "" {
				hookCmd := strings.ReplaceAll(watchOnChange, "{id}", event.DocID)
				hookCmd = strings.ReplaceAll(hookCmd, "{type}", event.Type)
				c := exec.CommandContext(ctx, "sh", "-c", hookCmd)
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				c.Run()
			}
		}

		return nil
	},
}

func init() {
	watchCmd.Flags().StringVar(&watchOnChange, "on-change", "", "command to exec on change ({id}, {type} placeholders)")
	rootCmd.AddCommand(watchCmd)
}
