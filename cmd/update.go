package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/itsfabioroma/remarkable-cli/pkg/update"
	"github.com/spf13/cobra"
)

// updateCmd self-updates the binary to match the latest main.
// Prefers in-place rebuild from the source repo when available; falls
// back to `go install @latest`.
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update remarkable to the latest version",
	Long: `Rebuild remarkable to match the latest commit on main.

Automatically detects how you installed:
 - If running from a git clone, runs git pull + go build in place
 - Otherwise runs go install github.com/itsfabioroma/remarkable-cli@latest

Your binary is swapped in place with no manual steps.`,
	Example: `  remarkable update
  remarkable update --check   # check only, no install`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// check what the latest release/commit is
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		latestTag, latestSHA, latestDate, err := update.ForceCheck(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not check GitHub: %v\n", err)
			return err
		}

		// label for the remote target: tag if a release exists, else short SHA
		remoteLabel := latestTag
		if remoteLabel == "" {
			remoteLabel = short(latestSHA)
		}
		datePrefix := ""
		if len(latestDate) >= 10 {
			datePrefix = latestDate[:10]
		}

		curSHA := update.CurrentSHA()
		if curSHA != "" && curSHA == latestSHA {
			fmt.Printf("remarkable is already up to date (%s)\n", remoteLabel)
			return nil
		}

		// is the remote actually ahead of us, or are we ahead (local commits)?
		remoteNewer := update.IsRemoteNewer(latestDate)

		if flagUpdateCheck {
			switch {
			case curSHA == "":
				fmt.Printf("latest: %s (%s)\n", remoteLabel, datePrefix)
			case remoteNewer:
				fmt.Printf("update available: %s → %s (%s)\n", short(curSHA), remoteLabel, datePrefix)
			default:
				fmt.Printf("you're ahead of upstream: local %s, remote %s\n", short(curSHA), remoteLabel)
			}
			return nil
		}

		if !remoteNewer && curSHA != "" {
			fmt.Printf("you're ahead of upstream — nothing to update (local %s, remote %s)\n", short(curSHA), remoteLabel)
			return nil
		}

		// do the update
		fmt.Fprintf(os.Stderr, "updating to %s ...\n", remoteLabel)
		status, err := update.SelfUpdate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			return err
		}
		fmt.Printf("✓ %s\n", status)
		fmt.Printf("  %s → %s\n", short(curSHA), short(latestSHA))
		return nil
	},
}

// --check makes the command just print version info without installing
var flagUpdateCheck bool

func short(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	if sha == "" {
		return "dev"
	}
	return sha
}

func init() {
	updateCmd.Flags().BoolVar(&flagUpdateCheck, "check", false, "check for updates without installing")
	rootCmd.AddCommand(updateCmd)
}
