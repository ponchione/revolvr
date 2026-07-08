package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"revolvr/internal/app"
	"revolvr/internal/ledger"
	"revolvr/internal/taskqueue"
)

const (
	defaultViewportWidth  = 80
	defaultViewportHeight = 24
)

var _ tea.Model = StatusModel{}

type StatusModel struct {
	status   app.StatusResult
	viewport viewport.Model
}

type RunOptions struct {
	Input  io.Reader
	Output io.Writer
}

func NewStatusModel(status app.StatusResult) StatusModel {
	model := StatusModel{
		status:   status,
		viewport: viewport.New(defaultViewportWidth, defaultViewportHeight),
	}
	model.viewport.SetContent(model.renderStatus())
	return model
}

func RunStatus(ctx context.Context, status app.StatusResult, opts RunOptions) error {
	options := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithInput(opts.Input),
	}
	if opts.Output != nil {
		options = append(options, tea.WithOutput(opts.Output))
	}

	_, err := tea.NewProgram(NewStatusModel(status), options...).Run()
	return err
}

func (m StatusModel) Init() tea.Cmd {
	return nil
}

func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m StatusModel) View() string {
	return m.viewport.View()
}

func (m StatusModel) renderStatus() string {
	if !m.status.Initialized {
		return lipgloss.JoinVertical(lipgloss.Left,
			"Revolvr",
			"State: not initialized",
		)
	}

	counts := countTasks(m.status.Tasks)
	lines := []string{
		"Revolvr",
		"State: initialized",
		"",
		"Tasks",
		fmt.Sprintf("Total: %d", counts.total),
		fmt.Sprintf("Pending: %d", counts.pending),
		fmt.Sprintf("Blocked: %d", counts.blocked),
		fmt.Sprintf("Completed: %d", counts.completed),
		"",
	}

	lines = append(lines, latestRunLines(m.status.RecentRuns)...)
	lines = append(lines, "")
	lines = append(lines, recentRunLines(m.status.RecentRuns)...)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

type taskCounts struct {
	total     int
	pending   int
	blocked   int
	completed int
}

func countTasks(tasks []taskqueue.Task) taskCounts {
	counts := taskCounts{total: len(tasks)}
	for _, task := range tasks {
		switch task.Status {
		case taskqueue.StatusPending:
			counts.pending++
		case taskqueue.StatusBlocked:
			counts.blocked++
		case taskqueue.StatusCompleted:
			counts.completed++
		}
	}
	return counts
}

func latestRunLines(runs []ledger.Run) []string {
	lines := []string{"Latest Run"}
	if len(runs) == 0 {
		return append(lines, "None")
	}

	run := runs[0]
	return append(lines,
		fmt.Sprintf("ID: %s", optionalValue(run.ID)),
		fmt.Sprintf("Status: %s", optionalValue(run.Status)),
		fmt.Sprintf("Summary: %s", optionalValue(run.Summary)),
		fmt.Sprintf("Verification: %s", optionalValue(run.VerificationStatus)),
		fmt.Sprintf("Commit: %s", optionalValue(run.CommitSHA)),
	)
}

func recentRunLines(runs []ledger.Run) []string {
	lines := []string{"Recent Runs"}
	if len(runs) == 0 {
		return append(lines, "None")
	}
	for _, run := range runs {
		summary := oneLine(run.Summary)
		if summary == "" {
			lines = append(lines, fmt.Sprintf("%s  %s", optionalValue(run.ID), optionalValue(run.Status)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %s  %s", optionalValue(run.ID), optionalValue(run.Status), summary))
	}
	return lines
}

func optionalValue(value string) string {
	value = oneLine(value)
	if value == "" {
		return "none"
	}
	return value
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
