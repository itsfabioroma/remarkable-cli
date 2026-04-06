package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/itsfabioroma/remarkable-cli/pkg/auth"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with reMarkable Cloud",
	Long:  "Register this device with reMarkable Cloud using a one-time code from my.remarkable.com",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := auth.NewTokenStore()

		// check if already authenticated
		tokens, err := auth.EnsureAuth(store)
		if err == nil {
			output(map[string]any{
				"status":   "authenticated",
				"deviceId": tokens.DeviceID,
			})
			if !flagJSON && isTerminal() {
				fmt.Println("Already authenticated with reMarkable Cloud.")
			}
			return nil
		}

		// prompt for one-time code
		if !flagJSON && isTerminal() {
			fmt.Println("To connect to reMarkable Cloud:")
			fmt.Println("1. Go to https://my.remarkable.com/device/browser/connect")
			fmt.Println("2. Enter the 8-character code below")
			fmt.Println()
		}

		fmt.Print("Code: ")
		reader := bufio.NewReader(os.Stdin)
		code, _ := reader.ReadString('\n')
		code = strings.TrimSpace(code)

		if len(code) == 0 {
			fmt.Fprintln(os.Stderr, "no code provided")
			return fmt.Errorf("no code provided")
		}

		// register device
		tokens, err = auth.RegisterDevice(code)
		if err != nil {
			outputError(err)
			return err
		}

		// save tokens
		if err := store.Save(tokens); err != nil {
			return err
		}

		output(map[string]any{
			"status":   "registered",
			"deviceId": tokens.DeviceID,
		})

		if !flagJSON && isTerminal() {
			fmt.Println("Successfully connected to reMarkable Cloud!")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
}
