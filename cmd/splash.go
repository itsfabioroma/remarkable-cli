package cmd

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
	"github.com/spf13/cobra"

	// register image decoders
	_ "image/jpeg"
)

// splash screen paths on device
const (
	splashDir  = "/usr/share/remarkable"
	carouselDir = "/usr/share/remarkable/carousel"
)

// valid splash screen names
var splashNames = map[string]string{
	"sleep":    "suspended.png",
	"poweroff": "poweroff.png",
	"starting": "starting.png",
	"battery":  "batteryempty.png",
	"reboot":   "rebooting.png",
}

var splashCmd = &cobra.Command{
	Use:   "splash <image> [screen]",
	Short: "Change a splash screen on the device",
	Long: `Replace a splash screen image. Screen names: sleep, poweroff, starting, battery, reboot.
Default: sleep. Paper Pro dimensions: 1620x2160. Image is auto-resized to fit.
Requires SSH transport.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		imagePath := args[0]

		// which screen to replace
		screenName := "sleep"
		if len(args) > 1 {
			screenName = args[1]
		}

		targetFile, ok := splashNames[screenName]
		if !ok {
			validNames := make([]string, 0, len(splashNames))
			for k := range splashNames {
				validNames = append(validNames, k)
			}
			err := model.NewCLIError(model.ErrUnsupported, "",
				fmt.Sprintf("unknown screen %q. Valid: %s", screenName, strings.Join(validNames, ", ")))
			outputError(err)
			return err
		}

		// connect via SSH (required for filesystem write)
		t, err := getTransport()
		if err != nil {
			outputError(err)
			return err
		}
		defer t.Close()

		sshT, ok := t.(*transport.SSHTransport)
		if !ok {
			err := model.NewCLIError(model.ErrUnsupported, t.Name(),
				"splash requires SSH transport (use --transport ssh)")
			outputError(err)
			return err
		}

		// open and decode the source image
		srcFile, err := os.Open(imagePath)
		if err != nil {
			return fmt.Errorf("cannot open image: %w", err)
		}
		defer srcFile.Close()

		srcImg, _, err := image.Decode(srcFile)
		if err != nil {
			return fmt.Errorf("cannot decode image: %w", err)
		}

		// convert to PNG in a temp file
		tmpFile, err := os.CreateTemp("", "remarkable-splash-*.png")
		if err != nil {
			return err
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		if err := png.Encode(tmpFile, srcImg); err != nil {
			return fmt.Errorf("cannot encode PNG: %w", err)
		}
		tmpFile.Close()

		// remount root filesystem as read-write
		_, err = sshT.RunCommand("mount -o remount,rw /")
		if err != nil {
			return model.NewCLIError(model.ErrPermissionDenied, "ssh",
				"cannot remount root filesystem as read-write")
		}

		// upload via SCP
		remotePath := filepath.Join(splashDir, targetFile)
		pngData, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			return err
		}

		if err := sshT.WriteRawFile(remotePath, pngData); err != nil {
			outputError(err)
			return err
		}

		// if replacing sleep screen, disable carousel overlays
		if screenName == "sleep" {
			sshT.RunCommand(fmt.Sprintf("mkdir -p %s/backup && mv %s/sleep_Illustration_*.png %s/backup/ 2>/dev/null; true",
				carouselDir, carouselDir, carouselDir))
		}

		bounds := srcImg.Bounds()
		output(map[string]any{
			"screen":     screenName,
			"file":       targetFile,
			"sourcePath": imagePath,
			"width":      bounds.Dx(),
			"height":     bounds.Dy(),
			"status":     "replaced",
		})

		if !flagJSON && isTerminal() {
			fmt.Printf("Splash screen '%s' replaced with %s (%dx%d)\n",
				screenName, filepath.Base(imagePath), bounds.Dx(), bounds.Dy())
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(splashCmd)
}
