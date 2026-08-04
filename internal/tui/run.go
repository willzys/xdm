package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
)

func Run(ctx context.Context, backend Backend) error {
	_, err := tea.NewProgram(New(ctx, backend), tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) {
		return nil
	}
	return err
}
