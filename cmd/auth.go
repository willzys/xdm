package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willzys/xdm/internal/auth"
	"github.com/willzys/xdm/internal/config"
)

var (
	authClientID    string
	authRedirectURI string
	authNoBrowser   bool
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Connect xdm to X using OAuth 2.0 PKCE",
	RunE:  runAuth,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the saved X OAuth token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := (auth.KeyringStore{}).Delete(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Signed out of X.")
		return nil
	},
}

func init() {
	authCmd.Flags().StringVar(&authClientID, "client-id", "", "OAuth 2.0 Client ID from the X Developer Console")
	authCmd.Flags().StringVar(&authRedirectURI, "redirect-uri", config.DefaultRedirectURI, "registered OAuth callback URI")
	authCmd.Flags().BoolVar(&authNoBrowser, "no-browser", false, "print the authorization URL without opening a browser")
	rootCmd.AddCommand(authCmd, logoutCmd)
}

func runAuth(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	clientID := strings.TrimSpace(authClientID)
	if clientID == "" {
		clientID = cfg.ClientID
	}
	if clientID == "" {
		return errors.New("--client-id is required the first time")
	}
	redirectURI := authRedirectURI
	if !cmd.Flags().Changed("redirect-uri") && cfg.RedirectURI != "" {
		redirectURI = cfg.RedirectURI
	}
	cfg.ClientID = clientID
	cfg.RedirectURI = redirectURI
	if err := config.Save(cfg); err != nil {
		return err
	}
	manager := auth.NewManager(clientID, redirectURI, auth.KeyringStore{})
	if err := manager.Authorize(cmd.Context(), !authNoBrowser); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Connected to X.")
	return nil
}
