package cmd

import (
	"fmt"
	"image/png"
	"os"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot [output.png]",
	Short: "Capture the device screen",
	Long: `Capture the current reMarkable screen as a PNG. Defaults to ./screenshot.png if no path is given.`,
	Example: `  remarkable screenshot
  remarkable screenshot /tmp/rm.png`,
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

		// create local PNG → wrap in CLIError envelope
		f, err := os.Create(outPath)
		if err != nil {
			code := model.ErrIO
			if os.IsNotExist(err) {
				code = model.ErrNotFound
			}
			e := model.NewCLIError(code, "", fmt.Sprintf("cannot create %s: %v", outPath, err))
			outputError(e)
			return e
		}
		defer f.Close()

		// encode PNG → wrap encode errors
		if err := png.Encode(f, img); err != nil {
			e := model.NewCLIError(model.ErrIO, "", fmt.Sprintf("cannot encode %s: %v", outPath, err))
			outputError(e)
			return e
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
