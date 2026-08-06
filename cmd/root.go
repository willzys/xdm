package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willzys/xdm/internal/api"
	"github.com/willzys/xdm/internal/auth"
	"github.com/willzys/xdm/internal/cache"
	"github.com/willzys/xdm/internal/config"
	"github.com/willzys/xdm/internal/service"
	"github.com/willzys/xdm/internal/tui"
	"github.com/willzys/xdm/internal/webapi"
	"github.com/willzys/xdm/internal/webauth"
)

var backend string

var rootCmd = &cobra.Command{
	Use:          "xdm",
	Short:        "Terminal-first Direct Messages client for X",
	SilenceUsage: true,
	RunE:         run,
}

func ExecuteContext(ctx context.Context) error { return rootCmd.ExecuteContext(ctx) }

func init() {
	rootCmd.Flags().StringVar(&backend, "backend", "official", "DM backend to use: official or web")
}

func run(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var client service.Client
	var path string
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "official":
		if cfg.ClientID == "" {
			return errors.New("xdm is not configured; run 'xdm auth --client-id <id>'")
		}
		if cfg.RedirectURI == "" {
			cfg.RedirectURI = config.DefaultRedirectURI
		}
		path, err = config.CachePath()
		if err != nil {
			return err
		}
		tokens := auth.NewManager(cfg.ClientID, cfg.RedirectURI, auth.KeyringStore{})
		client = api.NewClient(tokens)
	case "web":
		store, storeErr := webauth.NewStore()
		if storeErr != nil {
			return storeErr
		}
		session, loadErr := store.LoadActive()
		if loadErr != nil {
			return loadErr
		}
		httpClient, clientErr := webauth.NewHTTPClient(session, store)
		if clientErr != nil {
			return clientErr
		}
		client, clientErr = webapi.NewClient(httpClient, api.User{
			ID: session.UserID(), Name: session.Account.Name, Username: session.Account.Username,
		})
		if clientErr != nil {
			return clientErr
		}
		path, err = config.WebCachePath(session.Key())
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: the experimental web backend uses undocumented X endpoints and may cause account restrictions or violate X terms.")
	default:
		return fmt.Errorf("unknown backend %q; use official or web", backend)
	}
	messageCache, err := cache.Open(path)
	if err != nil {
		return fmt.Errorf("opening message cache: %w", err)
	}
	defer messageCache.Close()
	return tui.Run(cmd.Context(), service.New(client, messageCache))
}
