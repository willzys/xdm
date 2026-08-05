package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/willzys/xdm/internal/auth"
	"github.com/willzys/xdm/internal/config"
	"github.com/willzys/xdm/internal/webauth"
)

var (
	authClientID    string
	authRedirectURI string
	authNoBrowser   bool
	authWebBrowser  string
	authWebTimeout  time.Duration
	logoutAccount   string
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Connect xdm to X",
	Args:  cobra.NoArgs,
	RunE:  runAuth,
}

var logoutCmd = &cobra.Command{
	Use:   "logout [official|web|all]",
	Short: "Remove saved X authentication",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := "official"
		if len(args) == 1 {
			provider = strings.ToLower(args[0])
		}
		if logoutAccount != "" && provider != "web" {
			return errors.New("--account can only be used with 'xdm logout web'")
		}
		switch provider {
		case "official":
			if err := (auth.KeyringStore{}).Delete(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Signed out of the official X API.")
		case "web":
			store, err := webauth.NewStore()
			if err != nil {
				return err
			}
			if err := store.Delete(logoutAccount); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Removed the saved X web session.")
		case "all":
			if err := (auth.KeyringStore{}).Delete(); err != nil {
				return err
			}
			store, err := webauth.NewStore()
			if err != nil {
				return err
			}
			if err := store.Delete(""); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Removed all saved X authentication.")
		default:
			return fmt.Errorf("unknown authentication provider %q; use official, web, or all", provider)
		}
		return nil
	},
}

var authWebCmd = &cobra.Command{
	Use:   "web",
	Short: "Connect xdm to an X web session using an isolated browser profile",
	Args:  cobra.NoArgs,
	RunE:  runAuthWeb,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show saved X authentication without exposing credentials",
	Args:  cobra.NoArgs,
	RunE:  runAuthStatus,
}

var authUseCmd = &cobra.Command{
	Use:   "use <account>",
	Short: "Select the active saved X web account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := webauth.NewStore()
		if err != nil {
			return err
		}
		if err := store.SetActive(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Selected %s for X web access.\n", args[0])
		return nil
	},
}

func init() {
	authCmd.Flags().StringVar(&authClientID, "client-id", "", "OAuth 2.0 Client ID from the X Developer Console")
	authCmd.Flags().StringVar(&authRedirectURI, "redirect-uri", config.DefaultRedirectURI, "registered OAuth callback URI")
	authCmd.Flags().BoolVar(&authNoBrowser, "no-browser", false, "print the authorization URL without opening a browser")
	authWebCmd.Flags().StringVar(&authWebBrowser, "browser", "auto", "browser to use: auto, chrome, edge, or chromium")
	authWebCmd.Flags().DurationVar(&authWebTimeout, "timeout", 5*time.Minute, "maximum time to wait for browser login")
	logoutCmd.Flags().StringVar(&logoutAccount, "account", "", "remove only this saved web account")
	authCmd.AddCommand(authWebCmd, authStatusCmd, authUseCmd)
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

func runAuthWeb(cmd *cobra.Command, args []string) error {
	session, err := webauth.Login(cmd.Context(), webauth.LoginOptions{
		Browser: authWebBrowser,
		Timeout: authWebTimeout,
		Output:  cmd.OutOrStdout(),
	})
	if err != nil {
		return err
	}
	store, err := webauth.NewStore()
	if err != nil {
		return err
	}
	if err := store.Save(session); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Connected X web session as %s using %s.\n", session.DisplayName(), session.Browser)
	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	output := cmd.OutOrStdout()
	if token, err := (auth.KeyringStore{}).Load(); err == nil {
		state := "saved"
		if !token.ExpiresAt.IsZero() {
			if token.ExpiresAt.After(time.Now()) {
				state = "valid until " + token.ExpiresAt.Local().Format(time.RFC3339)
			} else if token.RefreshToken != "" {
				state = "access token expired; refresh token saved"
			} else {
				state = "expired"
			}
		}
		fmt.Fprintf(output, "Official: %s\n", state)
	} else if errors.Is(err, auth.ErrNotAuthenticated) {
		fmt.Fprintln(output, "Official: not connected")
	} else {
		return err
	}
	store, err := webauth.NewStore()
	if err != nil {
		return err
	}
	sessions, active, err := store.List()
	if errors.Is(err, webauth.ErrNotAuthenticated) {
		fmt.Fprintln(output, "Web: not connected")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "Web:")
	for _, session := range sessions {
		marker := " "
		if session.Key() == active {
			marker = "*"
		}
		state := "saved"
		if err := session.Validate(time.Now()); err != nil {
			state = "expired or incomplete"
		}
		fmt.Fprintf(output, "  %s %s (%s, %s)\n", marker, session.DisplayName(), session.Browser, state)
	}
	return nil
}
