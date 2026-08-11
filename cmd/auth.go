package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/willzys/xdm/internal/accountdata"
	"github.com/willzys/xdm/internal/api"
	"github.com/willzys/xdm/internal/auth"
	"github.com/willzys/xdm/internal/config"
	"github.com/willzys/xdm/internal/webapi"
	"github.com/willzys/xdm/internal/webauth"
	"github.com/willzys/xdm/internal/xchat"
)

var (
	authClientID     string
	authRedirectURI  string
	authNoBrowser    bool
	authWebBrowser   string
	authWebTimeout   time.Duration
	logoutAccount    string
	logoutDeleteData bool
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Connect xdm to X",
	Args:  cobra.NoArgs,
	RunE:  runAuth,
}

var logoutCmd = &cobra.Command{
	Use:   "logout [official|web|all]",
	Short: "Remove saved X authentication and optionally local account data",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runLogout,
}

var authWebCmd = &cobra.Command{
	Use:   "web",
	Short: "Connect xdm to an X web session using a dedicated browser profile",
	Long: "Connect xdm to an X web session using a dedicated, persistent browser profile. " +
		"The login window runs without remote debugging. After you sign in and close it, " +
		"xdm reopens the same profile briefly to capture the authenticated session.",
	Args: cobra.NoArgs,
	RunE: runAuthWeb,
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

var authWebDiagnoseCmd = &cobra.Command{
	Use:   "diagnose",
	Short: "Inspect the saved web inbox response without exposing message data",
	Args:  cobra.NoArgs,
	RunE:  runAuthWebDiagnose,
}

var authWebUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Verify the XChat PIN and encrypted inbox without exposing messages",
	Args:  cobra.NoArgs,
	RunE:  runAuthWebUnlock,
}

func init() {
	authCmd.Flags().StringVar(&authClientID, "client-id", "", "OAuth 2.0 Client ID from the X Developer Console")
	authCmd.Flags().StringVar(&authRedirectURI, "redirect-uri", config.DefaultRedirectURI, "registered OAuth callback URI")
	authCmd.Flags().BoolVar(&authNoBrowser, "no-browser", false, "print the authorization URL without opening a browser")
	authWebCmd.Flags().StringVar(&authWebBrowser, "browser", "auto", "browser to use: auto, chrome, edge, or chromium")
	authWebCmd.Flags().DurationVar(&authWebTimeout, "timeout", 5*time.Minute, "maximum time for browser login and session capture")
	logoutCmd.Flags().StringVar(&logoutAccount, "account", "", "remove only this saved web account")
	logoutCmd.Flags().BoolVar(&logoutDeleteData, "delete-data", false, "also delete cached messages and dedicated browser data")
	authCmd.AddCommand(authWebCmd, authStatusCmd, authUseCmd)
	authWebCmd.AddCommand(authWebDiagnoseCmd, authWebUnlockCmd)
	rootCmd.AddCommand(authCmd, logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	provider := "official"
	if len(args) == 1 {
		provider = strings.ToLower(args[0])
	}
	if logoutAccount != "" && provider != "web" {
		return errors.New("--account can only be used with 'xdm logout web'")
	}
	var remover *accountdata.Remover
	var err error
	if logoutDeleteData {
		remover, err = accountdata.NewRemover()
		if err != nil {
			return err
		}
	}
	switch provider {
	case "official":
		if logoutDeleteData {
			err = errors.Join(remover.RemoveOfficial(), (auth.KeyringStore{}).Delete())
		} else {
			err = (auth.KeyringStore{}).Delete()
		}
		if err != nil {
			return err
		}
		if logoutDeleteData {
			fmt.Fprintln(cmd.OutOrStdout(), "Signed out of the official X API and removed its cached messages.")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Signed out of the official X API.")
		}
	case "web":
		store, storeErr := webauth.NewStore()
		if storeErr != nil {
			return storeErr
		}
		accountKey := ""
		if logoutDeleteData && strings.TrimSpace(logoutAccount) != "" {
			session, loadErr := store.Load(logoutAccount)
			if loadErr != nil {
				return loadErr
			}
			accountKey = session.Key()
		}
		if logoutDeleteData {
			if accountKey == "" {
				err = remover.RemoveAllWeb()
			} else {
				err = remover.RemoveWeb(accountKey)
			}
		}
		err = errors.Join(err, store.Delete(logoutAccount))
		if err != nil {
			return err
		}
		if logoutDeleteData {
			fmt.Fprintln(cmd.OutOrStdout(), "Removed the saved X web session, cached messages, and dedicated browser data.")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Removed the saved X web session.")
		}
	case "all":
		store, storeErr := webauth.NewStore()
		if storeErr != nil {
			return storeErr
		}
		if logoutDeleteData {
			err = remover.RemoveAll()
		}
		err = errors.Join(err, (auth.KeyringStore{}).Delete(), store.Delete(""))
		if err != nil {
			return err
		}
		if logoutDeleteData {
			fmt.Fprintln(cmd.OutOrStdout(), "Removed all saved X authentication and local account data.")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Removed all saved X authentication.")
		}
	default:
		return fmt.Errorf("unknown authentication provider %q; use official, web, or all", provider)
	}
	return nil
}

func runAuthWebUnlock(cmd *cobra.Command, args []string) error {
	store, err := webauth.NewStore()
	if err != nil {
		return err
	}
	session, err := store.LoadActive()
	if err != nil {
		return err
	}
	httpClient, err := webauth.NewHTTPClient(session, store)
	if err != nil {
		return err
	}
	client, err := webapi.NewClient(httpClient, api.User{
		ID: session.UserID(), Name: session.Account.Name, Username: session.Account.Username,
	})
	if err != nil {
		return err
	}
	material, err := client.PrepareXChatUnlock(cmd.Context())
	if err != nil {
		return err
	}
	output := cmd.ErrOrStderr()
	pin, err := readXChatPIN(output)
	if err != nil {
		return err
	}
	defer clear(pin)
	diagnostics, err := xchat.Diagnose(cmd.Context(), material, pin)
	if err != nil {
		return fmt.Errorf("unlocking XChat: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "XChat unlocked; decrypted messages: %d; decoded events: %d; errors: %d\n",
		diagnostics.Messages, diagnostics.Events, diagnostics.Errors)
	return nil
}

func readXChatPIN(output io.Writer) ([]byte, error) {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return nil, errors.New("XChat PIN must be entered from an interactive terminal")
	}
	fmt.Fprintln(output, "Enter your existing XChat PIN. Do not guess: failed attempts are limited.")
	fmt.Fprint(output, "XChat PIN: ")
	pin, err := term.ReadPassword(os.Stdin.Fd())
	fmt.Fprintln(output)
	if err != nil {
		return nil, fmt.Errorf("reading XChat PIN: %w", err)
	}
	return pin, nil
}

func runAuthWebDiagnose(cmd *cobra.Command, args []string) error {
	store, err := webauth.NewStore()
	if err != nil {
		return err
	}
	session, err := store.LoadActive()
	if err != nil {
		return err
	}
	httpClient, err := webauth.NewHTTPClient(session, store)
	if err != nil {
		return err
	}
	client, err := webapi.NewClient(httpClient, api.User{
		ID: session.UserID(), Name: session.Account.Name, Username: session.Account.Username,
	})
	if err != nil {
		return err
	}
	diagnostics, err := client.DiagnoseInbox(cmd.Context())
	if err != nil {
		return err
	}
	output := cmd.OutOrStdout()
	fmt.Fprintf(output, "Initial state: %t\n", diagnostics.HasInitialState)
	fmt.Fprintf(output, "Conversations: %d; entries: %d; message entries: %d; users: %d\n",
		diagnostics.ConversationCount, diagnostics.EntryCount, diagnostics.MessageEntryCount, diagnostics.UserCount)
	fmt.Fprintf(output, "Top-level fields: %s\n", strings.Join(diagnostics.TopLevelFields, ", "))
	fmt.Fprintf(output, "Initial-state fields: %s\n", strings.Join(diagnostics.InitialStateFields, ", "))
	fmt.Fprintf(output, "XChat items: %d; encoded events: %d; key events: %d; errors: %d\n",
		diagnostics.XChatItemCount, diagnostics.XChatEventCount, diagnostics.XChatKeyEventCount, diagnostics.XChatErrorCount)
	fmt.Fprintf(output, "XChat messages: %d; encrypted: %d; plaintext: %d; decode failures: %d\n",
		diagnostics.XChatMessageCount, diagnostics.XChatEncryptedCount, diagnostics.XChatPlaintextCount, diagnostics.XChatDecodeFailures)
	fmt.Fprintf(output, "XChat key versions: %d; Juicebox realms: %d; config: %t; managed PIN: %t\n",
		diagnostics.XChatPublicKeyVersions, diagnostics.XChatJuiceboxRealms,
		diagnostics.XChatHasJuiceboxConfig, diagnostics.XChatManagedPIN)
	if len(diagnostics.EntryKinds) > 0 {
		kinds := make([]string, 0, len(diagnostics.EntryKinds))
		for kind, count := range diagnostics.EntryKinds {
			kinds = append(kinds, fmt.Sprintf("%s=%d", kind, count))
		}
		sort.Strings(kinds)
		fmt.Fprintf(output, "Entry kinds: %s\n", strings.Join(kinds, ", "))
	}
	return nil
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
