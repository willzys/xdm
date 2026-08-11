package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willzys/xdm/internal/accountdata"
	"github.com/willzys/xdm/internal/webauth"
)

var cacheAccount string

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage locally cached messages",
	Args:  cobra.NoArgs,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear [official|web|all]",
	Short: "Delete locally cached messages without removing authentication",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCacheClear,
}

func init() {
	cacheClearCmd.Flags().StringVar(&cacheAccount, "account", "", "clear cache for this web account")
	cacheCmd.AddCommand(cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}

func runCacheClear(cmd *cobra.Command, args []string) error {
	provider := "official"
	if len(args) == 1 {
		provider = strings.ToLower(args[0])
	}
	if cacheAccount != "" && provider != "web" {
		return errors.New("--account can only be used with 'xdm cache clear web'")
	}
	remover, err := accountdata.NewRemover()
	if err != nil {
		return err
	}
	switch provider {
	case "official":
		err = remover.RemoveOfficial()
	case "web":
		if strings.TrimSpace(cacheAccount) == "" {
			return errors.New("--account is required with 'xdm cache clear web'; use 'xdm cache clear all' to clear every cache")
		}
		accountKey := ""
		if store, storeErr := webauth.NewStore(); storeErr == nil {
			if session, loadErr := store.Load(cacheAccount); loadErr == nil {
				accountKey = session.Key()
			}
		}
		if accountKey == "" {
			accountKey, err = remover.ResolveWebCache(cacheAccount)
			if err != nil {
				return err
			}
		}
		err = remover.RemoveWebCache(accountKey)
	case "all":
		err = errors.Join(remover.RemoveOfficial(), remover.RemoveAllWebCaches())
	default:
		return fmt.Errorf("unknown cache provider %q; use official, web, or all", provider)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Removed cached messages. Authentication and dedicated browser data were preserved.")
	return nil
}
