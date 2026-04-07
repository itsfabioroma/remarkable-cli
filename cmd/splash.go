package cmd

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/model"
	"github.com/spf13/cobra"

	// register image decoders
	_ "image/jpeg"
)

// device paths
const (
	splashDir   = "/usr/share/remarkable"
	carouselDir = "/usr/share/remarkable/carousel"
	backupDir   = "/usr/share/remarkable/.splash-backup"
)

// Paper Pro dimensions (portrait)
const (
	ppSplashW = 1620
	ppSplashH = 2160
)

// splash screen names → filenames on device
var splashNames = map[string]string{
	"sleep":    "suspended.png",
	"poweroff": "poweroff.png",
	"starting": "starting.png",
	"battery":  "batteryempty.png",
	"reboot":   "rebooting.png",
}

var splashCmd = &cobra.Command{
	Use:   "splash",
	Short: "Manage device splash screens",
	Long: `Manage the splash screens shown during sleep, power off, boot, etc. Use the subcommands set, list, restore.`,
	Example: `  remarkable splash list
  remarkable splash set art.png
  remarkable splash restore`,
}

// --- splash set ---

var splashSetCmd = &cobra.Command{
	Use:   "set <image> [screen]",
	Short: "Replace a splash screen",
	Long: `Replace a splash screen. Auto-resizes to fit Paper Pro (1620x2160), backs up the original first. Accepts PNG and JPG.

Screens: sleep (default), poweroff, starting, battery, reboot.`,
	Example: `  remarkable splash set art.png
  remarkable splash set logo.jpg poweroff`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		imagePath := args[0]

		// which screen
		screenName := "sleep"
		if len(args) > 1 {
			screenName = args[1]
		}

		targetFile, ok := splashNames[screenName]
		if !ok {
			names := sortedKeys(splashNames)
			err := model.NewCLIError(model.ErrUnsupported, "",
				fmt.Sprintf("unknown screen %q. Valid: %s", screenName, strings.Join(names, ", ")))
			outputError(err)
			return err
		}

		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		// decode source image → wrap in CLIError envelope
		srcFile, err := os.Open(imagePath)
		if err != nil {
			code := model.ErrIO
			if os.IsNotExist(err) {
				code = model.ErrNotFound
			}
			e := model.NewCLIError(code, "", fmt.Sprintf("cannot read %s: %v", imagePath, err))
			outputError(e)
			return e
		}
		defer srcFile.Close()

		// decode image bytes → wrap decode errors
		srcImg, _, err := image.Decode(srcFile)
		if err != nil {
			e := model.NewCLIError(model.ErrUnsupported, "", fmt.Sprintf("cannot decode %s: %v", imagePath, err))
			outputError(e)
			return e
		}

		// resize to device dimensions (fit, center, white background)
		resized := fitToScreen(srcImg, ppSplashW, ppSplashH)

		// encode to PNG → wrap encode errors
		var buf bytes.Buffer
		if err := png.Encode(&buf, resized); err != nil {
			e := model.NewCLIError(model.ErrIO, "", fmt.Sprintf("cannot encode PNG: %v", err))
			outputError(e)
			return e
		}

		// remount rw + backup original
		sshT.RunCommand("mount -o remount,rw /")
		remotePath := filepath.Join(splashDir, targetFile)
		sshT.RunCommand(fmt.Sprintf("mkdir -p %s && cp -n %s %s/ 2>/dev/null; true", backupDir, remotePath, backupDir))

		// upload
		if err := sshT.WriteRawFile(remotePath, buf.Bytes()); err != nil {
			outputError(err)
			return err
		}

		// disable carousel overlay for sleep screen
		if screenName == "sleep" {
			sshT.RunCommand(fmt.Sprintf("mkdir -p %s/.backup && mv %s/sleep_Illustration_*.png %s/.backup/ 2>/dev/null; true",
				carouselDir, carouselDir, carouselDir))
		}

		bounds := resized.Bounds()
		output(map[string]any{
			"screen":  screenName,
			"file":    targetFile,
			"source":  filepath.Base(imagePath),
			"width":   bounds.Dx(),
			"height":  bounds.Dy(),
			"status":  "replaced",
		})

		if !flagJSON && isTerminal() {
			fmt.Printf("Set '%s' splash to %s (resized to %dx%d)\n",
				screenName, filepath.Base(imagePath), bounds.Dx(), bounds.Dy())
		}

		return nil
	},
}

// --- splash list ---

var splashListCmd = &cobra.Command{
	Use:   "list",
	Short: "List current splash screens on device",
	Long: `List every splash screen file on the device, whether it exists, and whether a backup of the original is available.`,
	Example: `  remarkable splash list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		// list splash files with sizes
		raw, _ := sshT.RunCommand(fmt.Sprintf("ls -la %s/*.png 2>/dev/null", splashDir))

		var screens []map[string]any
		for name, file := range splashNames {
			info := map[string]any{"screen": name, "file": file, "exists": false}

			// check if file exists in ls output
			if strings.Contains(raw, file) {
				info["exists"] = true
			}

			// check if backup exists
			backupRaw, _ := sshT.RunCommand(fmt.Sprintf("test -f %s/%s && echo yes || echo no", backupDir, file))
			info["hasBackup"] = strings.TrimSpace(backupRaw) == "yes"

			screens = append(screens, info)
		}

		output(screens)
		return nil
	},
}

// --- splash restore ---

var splashRestoreCmd = &cobra.Command{
	Use:   "restore [screen]",
	Short: "Restore original splash screen from backup",
	Long: `Restore the original factory splash screen from the backup made when you first ran "splash set".`,
	Example: `  remarkable splash restore
  remarkable splash restore poweroff`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		screenName := "sleep"
		if len(args) > 0 {
			screenName = args[0]
		}

		targetFile, ok := splashNames[screenName]
		if !ok {
			return fmt.Errorf("unknown screen: %s", screenName)
		}

		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		sshT.RunCommand("mount -o remount,rw /")

		// restore from backup
		backupPath := filepath.Join(backupDir, targetFile)
		remotePath := filepath.Join(splashDir, targetFile)

		check, _ := sshT.RunCommand(fmt.Sprintf("test -f %s && echo yes || echo no", backupPath))
		if strings.TrimSpace(check) != "yes" {
			err := model.NewCLIError(model.ErrNotFound, "ssh",
				fmt.Sprintf("no backup found for '%s'", screenName))
			outputError(err)
			return err
		}

		sshT.RunCommand(fmt.Sprintf("cp %s %s", backupPath, remotePath))

		// restore carousel if sleep
		if screenName == "sleep" {
			sshT.RunCommand(fmt.Sprintf("mv %s/.backup/sleep_Illustration_*.png %s/ 2>/dev/null; true",
				carouselDir, carouselDir))
		}

		output(map[string]any{
			"screen": screenName,
			"status": "restored",
		})

		if !flagJSON && isTerminal() {
			fmt.Printf("Restored original '%s' splash screen\n", screenName)
		}

		return nil
	},
}

// --- helpers ---

// fitToScreen resizes an image to fit within target dimensions, centered on white background
func fitToScreen(src image.Image, targetW, targetH int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// if already correct size, return as-is
	if srcW == targetW && srcH == targetH {
		return src
	}

	// calculate scale to fit (maintain aspect ratio)
	scaleW := float64(targetW) / float64(srcW)
	scaleH := float64(targetH) / float64(srcH)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)

	// simple nearest-neighbor resize
	resized := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			srcX := int(float64(x) / scale)
			srcY := int(float64(y) / scale)
			if srcX >= srcW {
				srcX = srcW - 1
			}
			if srcY >= srcH {
				srcY = srcH - 1
			}
			resized.Set(x, y, src.At(srcBounds.Min.X+srcX, srcBounds.Min.Y+srcY))
		}
	}

	// center on white background
	canvas := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	offsetX := (targetW - newW) / 2
	offsetY := (targetH - newH) / 2
	draw.Draw(canvas, image.Rect(offsetX, offsetY, offsetX+newW, offsetY+newH),
		resized, image.Point{}, draw.Over)

	return canvas
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func init() {
	splashCmd.AddCommand(splashSetCmd)
	splashCmd.AddCommand(splashListCmd)
	splashCmd.AddCommand(splashRestoreCmd)
	rootCmd.AddCommand(splashCmd)
}
