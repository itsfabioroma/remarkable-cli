package cmd

import (
	"fmt"
	"image/png"
	"os"

	"github.com/spf13/cobra"
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot [output.png]",
	Short: "Capture the device screen",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		// capture
		img, err := sshT.Screenshot()
		if err != nil {
			outputError(err)
			return err
		}

		// write PNG
		outPath := "screenshot.png"
		if len(args) > 0 {
			outPath = args[0]
		}

		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("cannot create %s: %w", outPath, err)
		}
		defer f.Close()

		if err := png.Encode(f, img); err != nil {
			return fmt.Errorf("cannot encode PNG: %w", err)
		}

		output(map[string]any{
			"file":   outPath,
			"width":  img.Bounds().Dx(),
			"height": img.Bounds().Dy(),
			"status": "captured",
		})

		if !flagJSON && isTerminal() {
			fmt.Printf("Screenshot saved to %s (%dx%d)\n",
				outPath, img.Bounds().Dx(), img.Bounds().Dy())
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(screenshotCmd)
}
