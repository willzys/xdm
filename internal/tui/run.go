package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(ctx context.Context, backend Backend) error {
	_, err := tea.NewProgram(New(ctx, backend), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) {
		return nil
	}
	return err
}
