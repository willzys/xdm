package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/willzys/xdm/internal/api"
	"github.com/willzys/xdm/internal/auth"
	"github.com/willzys/xdm/internal/cache"
	"github.com/willzys/xdm/internal/config"
	"github.com/willzys/xdm/internal/service"
	"github.com/willzys/xdm/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:          "xdm",
	Short:        "Terminal-first Direct Messages client for X",
	SilenceUsage: true,
	RunE:         run,
}

func ExecuteContext(ctx context.Context) error { return rootCmd.ExecuteContext(ctx) }

func run(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.ClientID == "" {
		return errors.New("xdm is not configured; run 'xdm auth --client-id <id>'")
	}
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = config.DefaultRedirectURI
	}
	path, err := config.CachePath()
	if err != nil {
		return err
	}
	messageCache, err := cache.Open(path)
	if err != nil {
		return fmt.Errorf("opening message cache: %w", err)
	}
	defer messageCache.Close()
	tokens := auth.NewManager(cfg.ClientID, cfg.RedirectURI, auth.KeyringStore{})
	client := api.NewClient(tokens)
	return tui.Run(cmd.Context(), service.New(client, messageCache))
}
