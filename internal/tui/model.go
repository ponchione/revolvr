package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

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
	status      app.StatusResult
	actions     StatusActions
	selectedRun int
	runDetails  *ledger.RunWithEvents
	message     string
	viewport    viewport.Model
}

type RefreshStatusFunc func() (app.StatusResult, error)
type OpenRunFunc func(runID string) (ledger.RunWithEvents, error)

type StatusActions struct {
	RefreshStatus RefreshStatusFunc
	OpenRun       OpenRunFunc
}

type RunOptions struct {
	Input         io.Reader
	Output        io.Writer
	RefreshStatus RefreshStatusFunc
	OpenRun       OpenRunFunc
}

func NewStatusModel(status app.StatusResult) StatusModel {
	return NewStatusModelWithActions(status, StatusActions{})
}

func NewStatusModelWithActions(status app.StatusResult, actions StatusActions) StatusModel {
	model := StatusModel{
		status:      status,
		actions:     actions,
		selectedRun: clampRunIndex(status.RecentRuns, 0),
		viewport:    viewport.New(defaultViewportWidth, defaultViewportHeight),
	}
	model.updateViewportContent()
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

	_, err := tea.NewProgram(NewStatusModelWithActions(status, StatusActions{
		RefreshStatus: opts.RefreshStatus,
		OpenRun:       opts.OpenRun,
	}), options...).Run()
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
	case refreshStatusMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Refresh failed: %s", msg.err)
		} else {
			selectedID := m.selectedRunID()
			m.status = msg.status
			m.selectedRun = selectedRunIndex(m.status.RecentRuns, selectedID)
			m.runDetails = nil
			m.message = "Refreshed."
		}
		m.updateViewportContent()
		return m, nil
	case openRunMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Open failed: %s", msg.err)
		} else {
			m.runDetails = &msg.history
			m.message = ""
		}
		m.updateViewportContent()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			if m.actions.RefreshStatus == nil {
				m.message = "Refresh is unavailable."
				m.updateViewportContent()
				return m, nil
			}
			return m, m.refreshStatusCmd()
		case "enter", "o":
			if m.actions.OpenRun == nil {
				m.message = "Open is unavailable."
				m.updateViewportContent()
				return m, nil
			}
			cmd := m.openSelectedRunCmd()
			if cmd == nil {
				m.message = "No run selected."
				m.updateViewportContent()
				return m, nil
			}
			return m, cmd
		case "up", "k":
			m.moveSelectedRun(-1)
			m.runDetails = nil
			m.updateViewportContent()
			return m, nil
		case "down", "j":
			m.moveSelectedRun(1)
			m.runDetails = nil
			m.updateViewportContent()
			return m, nil
		case "esc", "backspace":
			if m.runDetails != nil {
				m.runDetails = nil
				m.updateViewportContent()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m StatusModel) View() string {
	return m.viewport.View()
}

type refreshStatusMsg struct {
	status app.StatusResult
	err    error
}

type openRunMsg struct {
	history ledger.RunWithEvents
	err     error
}

func (m StatusModel) refreshStatusCmd() tea.Cmd {
	return func() tea.Msg {
		status, err := m.actions.RefreshStatus()
		return refreshStatusMsg{status: status, err: err}
	}
}

func (m StatusModel) openSelectedRunCmd() tea.Cmd {
	if len(m.status.RecentRuns) == 0 {
		return nil
	}
	index := clampRunIndex(m.status.RecentRuns, m.selectedRun)
	runID := strings.TrimSpace(m.status.RecentRuns[index].ID)
	if runID == "" {
		return nil
	}
	return func() tea.Msg {
		history, err := m.actions.OpenRun(runID)
		return openRunMsg{history: history, err: err}
	}
}

func (m *StatusModel) updateViewportContent() {
	m.viewport.SetContent(m.render())
	m.viewport.GotoTop()
}

func (m StatusModel) render() string {
	if m.runDetails != nil {
		return m.renderRunDetails(*m.runDetails)
	}
	return m.renderStatus()
}

func (m StatusModel) renderStatus() string {
	if !m.status.Initialized {
		lines := []string{
			"Revolvr",
			"State: not initialized",
		}
		lines = appendNotice(lines, m.message)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
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
	lines = appendNotice(lines, m.message)

	lines = append(lines, latestRunLines(m.status.RecentRuns)...)
	lines = append(lines, "")
	lines = append(lines, m.recentRunLines()...)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderRunDetails(history ledger.RunWithEvents) string {
	run := history.Run
	lines := []string{
		"Run Details",
		fmt.Sprintf("ID: %s", optionalValue(run.ID)),
		fmt.Sprintf("Task ID: %s", optionalValue(run.TaskID)),
		fmt.Sprintf("Task: %s", optionalValue(run.Task)),
		fmt.Sprintf("Status: %s", optionalValue(run.Status)),
		fmt.Sprintf("Summary: %s", optionalValue(run.Summary)),
		fmt.Sprintf("Started: %s", optionalTime(run.StartedAt)),
		fmt.Sprintf("Completed: %s", optionalTimePtr(run.CompletedAt)),
		fmt.Sprintf("Codex exit code: %s", optionalIntPtr(run.CodexExitCode)),
		fmt.Sprintf("Verification: %s", optionalValue(run.VerificationStatus)),
		fmt.Sprintf("Commit: %s", optionalValue(run.CommitSHA)),
		"",
	}

	lines = append(lines, runArtifactLines(history.Events)...)
	lines = append(lines, "")
	lines = append(lines, runEventLines(history.Events)...)
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

func (m StatusModel) recentRunLines() []string {
	lines := []string{"Recent Runs"}
	if len(m.status.RecentRuns) == 0 {
		return append(lines, "None")
	}
	selected := clampRunIndex(m.status.RecentRuns, m.selectedRun)
	for i, run := range m.status.RecentRuns {
		prefix := " "
		if i == selected {
			prefix = ">"
		}
		summary := oneLine(run.Summary)
		if summary == "" {
			lines = append(lines, fmt.Sprintf("%s %s  %s", prefix, optionalValue(run.ID), optionalValue(run.Status)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s  %s  %s", prefix, optionalValue(run.ID), optionalValue(run.Status), summary))
	}
	return lines
}

func runArtifactLines(events []ledger.Event) []string {
	lines := []string{"Artifacts"}
	artifacts, found := ledger.RunArtifactsFromEvents(events)
	if !found || artifacts.Empty() {
		return append(lines, "None")
	}
	for _, artifact := range []struct {
		label string
		path  string
	}{
		{label: "prompt", path: artifacts.PromptPath},
		{label: "codex stdout jsonl", path: artifacts.CodexStdoutJSONLPath},
		{label: "codex stderr", path: artifacts.CodexStderrPath},
		{label: "last message", path: artifacts.LastMessagePath},
		{label: "receipt", path: artifacts.ReceiptPath},
	} {
		if strings.TrimSpace(artifact.path) == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", artifact.label, artifact.path))
	}
	return lines
}

func runEventLines(events []ledger.Event) []string {
	lines := []string{"Events"}
	if len(events) == 0 {
		return append(lines, "None")
	}
	for _, event := range events {
		lines = append(lines, fmt.Sprintf("%d  %s  %s", event.ID, event.Type, optionalTime(event.CreatedAt)))
	}
	return lines
}

func appendNotice(lines []string, message string) []string {
	message = oneLine(message)
	if message == "" {
		return lines
	}
	return append(lines, "Notice: "+message, "")
}

func (m *StatusModel) moveSelectedRun(delta int) {
	if len(m.status.RecentRuns) == 0 {
		m.selectedRun = 0
		return
	}
	m.selectedRun = clampRunIndex(m.status.RecentRuns, m.selectedRun+delta)
}

func (m StatusModel) selectedRunID() string {
	if len(m.status.RecentRuns) == 0 {
		return ""
	}
	return strings.TrimSpace(m.status.RecentRuns[clampRunIndex(m.status.RecentRuns, m.selectedRun)].ID)
}

func selectedRunIndex(runs []ledger.Run, runID string) int {
	runID = strings.TrimSpace(runID)
	if runID != "" {
		for i, run := range runs {
			if strings.TrimSpace(run.ID) == runID {
				return i
			}
		}
	}
	return clampRunIndex(runs, 0)
}

func clampRunIndex(runs []ledger.Run, index int) int {
	if len(runs) == 0 || index < 0 {
		return 0
	}
	if index >= len(runs) {
		return len(runs) - 1
	}
	return index
}

func optionalValue(value string) string {
	value = oneLine(value)
	if value == "" {
		return "none"
	}
	return value
}

func optionalTime(value time.Time) string {
	if value.IsZero() {
		return "none"
	}
	return value.UTC().Format(time.RFC3339)
}

func optionalTimePtr(value *time.Time) string {
	if value == nil {
		return "none"
	}
	return optionalTime(*value)
}

func optionalIntPtr(value *int) string {
	if value == nil {
		return "none"
	}
	return fmt.Sprintf("%d", *value)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
