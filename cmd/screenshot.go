package cmd

import (
	"fmt"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot [output.png]",
	Short: "Capture the device screen",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		// must be a DeviceTransport (SSH)
		dt, ok := t.(transport.DeviceTransport)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"screenshot requires SSH transport (use --transport ssh)")
			outputError(err)
			return err
		}

		// capture
		img, err := dt.Screenshot()
		if err != nil {
			outputError(err)
			return err
		}

		// output path
		outPath := "screenshot.png"
		if len(args) > 0 {
			outPath = args[0]
		}

		if err := transport.SaveScreenshot(img, outPath); err != nil {
			return err
		}

		bounds := img.Bounds()
		output(map[string]any{
			"path":      outPath,
			"width":     bounds.Dx(),
			"height":    bounds.Dy(),
			"timestamp": time.Now().Format(time.RFC3339),
		})

		if !flagJSON && isTerminal() {
			fmt.Printf("Screenshot saved to %s (%dx%d)\n", outPath, bounds.Dx(), bounds.Dy())
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(screenshotCmd)
}
