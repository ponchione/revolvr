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

type TUIView int

const (
	viewDashboard TUIView = iota
	viewTasks
	viewRuns
	viewRunDetail
	viewHelp
	viewTaskEntry
)

type StatusModel struct {
	status       app.StatusResult
	actions      StatusActions
	view         TUIView
	previous     TUIView
	selectedTask int
	selectedRun  int
	runDetails   *ledger.RunWithEvents
	taskEntry    taskEntryState
	message      string
	width        int
	height       int
	viewport     viewport.Model
}

type RefreshStatusFunc func() (app.StatusResult, error)
type OpenRunFunc func(runID string) (ledger.RunWithEvents, error)
type AddTaskFunc func(input app.AddTaskInput) (taskqueue.Task, error)

type StatusActions struct {
	RefreshStatus RefreshStatusFunc
	OpenRun       OpenRunFunc
	AddTask       AddTaskFunc
}

type RunOptions struct {
	Input         io.Reader
	Output        io.Writer
	RefreshStatus RefreshStatusFunc
	OpenRun       OpenRunFunc
	AddTask       AddTaskFunc
}

type taskEntryField int

const (
	taskEntryTaskField taskEntryField = iota
	taskEntrySummaryField
)

type taskEntryState struct {
	previous TUIView
	field    taskEntryField
	taskText string
	summary  string
	message  string
}

func NewStatusModel(status app.StatusResult) StatusModel {
	return NewStatusModelWithActions(status, StatusActions{})
}

func NewStatusModelWithActions(status app.StatusResult, actions StatusActions) StatusModel {
	model := StatusModel{
		status:       status,
		actions:      actions,
		view:         viewDashboard,
		previous:     viewDashboard,
		selectedTask: clampTaskIndex(status.Tasks, 0),
		selectedRun:  clampRunIndex(status.RecentRuns, 0),
		width:        defaultViewportWidth,
		height:       defaultViewportHeight,
		viewport:     viewport.New(defaultViewportWidth, defaultViewportHeight),
	}
	model.resizeViewport()
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
		AddTask:       opts.AddTask,
	}), options...).Run()
	return err
}

func (m StatusModel) Init() tea.Cmd {
	return nil
}

func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		m.refreshViewportContent()
	case refreshStatusMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Refresh failed: %s", msg.err)
		} else {
			selectedTaskID := m.selectedTaskID()
			selectedID := m.selectedRunID()
			m.status = msg.status
			m.selectedTask = selectedTaskIndex(m.status.Tasks, selectedTaskID)
			m.selectedRun = selectedRunIndex(m.status.RecentRuns, selectedID)
			if !m.status.Initialized {
				m.runDetails = nil
			}
			m.message = "Refreshed."
		}
		m.updateViewportContent()
		return m, nil
	case openRunMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Open failed: %s", msg.err)
		} else {
			m.runDetails = &msg.history
			m.view = viewRunDetail
			m.message = ""
		}
		m.resizeViewport()
		m.updateViewportContent()
		return m, nil
	case addTaskMsg:
		if msg.err != nil {
			m.taskEntry.message = fmt.Sprintf("Add failed: %s", msg.err)
		} else {
			selectedRunID := m.selectedRunID()
			m.status = msg.status
			m.selectedTask = selectedTaskIndex(m.status.Tasks, msg.task.ID)
			m.selectedRun = selectedRunIndex(m.status.RecentRuns, selectedRunID)
			if !m.status.Initialized {
				m.runDetails = nil
			}
			m.view = viewTasks
			m.taskEntry = taskEntryState{}
			m.message = fmt.Sprintf("Added task %s.", optionalValue(msg.task.ID))
		}
		m.resizeViewport()
		m.updateViewportContent()
		return m, nil
	case tea.KeyMsg:
		if m.view == viewTaskEntry {
			return m.updateTaskEntry(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "1":
			m.switchView(viewDashboard)
			return m, nil
		case "2":
			m.switchView(viewTasks)
			return m, nil
		case "3":
			m.switchView(viewRuns)
			return m, nil
		case "4":
			m.switchView(viewRunDetail)
			return m, nil
		case "?":
			m.switchView(viewHelp)
			return m, nil
		case "a":
			m.startTaskEntry()
			return m, nil
		case "r":
			if m.actions.RefreshStatus == nil {
				m.message = "Refresh is unavailable."
				m.updateViewportContent()
				return m, nil
			}
			return m, m.refreshStatusCmd()
		case "esc", "backspace":
			switch m.view {
			case viewHelp:
				m.switchView(m.previous)
				return m, nil
			case viewRunDetail:
				m.switchView(viewRuns)
				return m, nil
			}
		}

		switch m.view {
		case viewTasks:
			switch msg.String() {
			case "up", "k":
				m.moveSelectedTask(-1)
				m.updateViewportContent()
				return m, nil
			case "down", "j":
				m.moveSelectedTask(1)
				m.updateViewportContent()
				return m, nil
			}
		case viewRuns:
			switch msg.String() {
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
				m.updateViewportContent()
				return m, nil
			case "down", "j":
				m.moveSelectedRun(1)
				m.updateViewportContent()
				return m, nil
			}
		case viewRunDetail:
			switch msg.String() {
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
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m StatusModel) View() string {
	sections := append([]string{}, m.headerLines()...)
	sections = append(sections, "")
	sections = append(sections, trimTrailingBlankLines(m.viewport.View()))
	sections = append(sections, "")
	sections = append(sections, m.footerLines()...)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

type refreshStatusMsg struct {
	status app.StatusResult
	err    error
}

type openRunMsg struct {
	history ledger.RunWithEvents
	err     error
}

type addTaskMsg struct {
	task   taskqueue.Task
	status app.StatusResult
	err    error
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

func (m StatusModel) addTaskCmd(input app.AddTaskInput) tea.Cmd {
	return func() tea.Msg {
		task, err := m.actions.AddTask(input)
		if err != nil {
			return addTaskMsg{err: err}
		}
		status, err := m.actions.RefreshStatus()
		if err != nil {
			return addTaskMsg{task: task, err: fmt.Errorf("refresh after add: %w", err)}
		}
		return addTaskMsg{task: task, status: status}
	}
}

func (m StatusModel) updateTaskEntry(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.cancelTaskEntry()
		return m, nil
	case "enter":
		input := app.AddTaskInput{
			Task:    strings.TrimSpace(m.taskEntry.taskText),
			Summary: strings.TrimSpace(m.taskEntry.summary),
		}
		if input.Task == "" {
			m.taskEntry.message = "Task text is required."
			m.updateViewportContent()
			return m, nil
		}
		if m.actions.AddTask == nil {
			m.taskEntry.message = "Add task is unavailable."
			m.updateViewportContent()
			return m, nil
		}
		if m.actions.RefreshStatus == nil {
			m.taskEntry.message = "Refresh is unavailable."
			m.updateViewportContent()
			return m, nil
		}
		m.taskEntry.message = ""
		m.updateViewportContent()
		return m, m.addTaskCmd(input)
	case "tab", "shift+tab":
		m.toggleTaskEntryField()
		m.taskEntry.message = ""
		m.updateViewportContent()
		return m, nil
	case "backspace":
		m.backspaceTaskEntry()
		m.taskEntry.message = ""
		m.updateViewportContent()
		return m, nil
	}

	switch msg.Type {
	case tea.KeyRunes:
		m.appendTaskEntryRunes(msg.Runes)
	case tea.KeySpace:
		m.appendTaskEntryRunes([]rune(" "))
	default:
		return m, nil
	}
	m.taskEntry.message = ""
	m.updateViewportContent()
	return m, nil
}

func (m *StatusModel) updateViewportContent() {
	m.viewport.SetContent(m.renderContent())
	m.viewport.GotoTop()
}

func (m *StatusModel) refreshViewportContent() {
	m.viewport.SetContent(m.renderContent())
}

func (m *StatusModel) switchView(view TUIView) {
	if m.view != view && view != viewHelp {
		m.previous = m.view
	}
	if view == viewHelp && m.view != viewHelp {
		m.previous = m.view
	}
	m.view = view
	m.resizeViewport()
	m.updateViewportContent()
}

func (m *StatusModel) startTaskEntry() {
	m.taskEntry = taskEntryState{
		previous: m.view,
		field:    taskEntryTaskField,
	}
	m.view = viewTaskEntry
	m.message = ""
	m.resizeViewport()
	m.updateViewportContent()
}

func (m *StatusModel) cancelTaskEntry() {
	previous := m.taskEntry.previous
	if previous == viewTaskEntry {
		previous = viewTasks
	}
	m.view = previous
	m.taskEntry = taskEntryState{}
	m.resizeViewport()
	m.updateViewportContent()
}

func (m *StatusModel) toggleTaskEntryField() {
	if m.taskEntry.field == taskEntryTaskField {
		m.taskEntry.field = taskEntrySummaryField
		return
	}
	m.taskEntry.field = taskEntryTaskField
}

func (m *StatusModel) appendTaskEntryRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	switch m.taskEntry.field {
	case taskEntrySummaryField:
		m.taskEntry.summary += string(runes)
	default:
		m.taskEntry.taskText += string(runes)
	}
}

func (m *StatusModel) backspaceTaskEntry() {
	switch m.taskEntry.field {
	case taskEntrySummaryField:
		m.taskEntry.summary = trimLastRune(m.taskEntry.summary)
	default:
		m.taskEntry.taskText = trimLastRune(m.taskEntry.taskText)
	}
}

func (m *StatusModel) resizeViewport() {
	width := m.width
	if width <= 0 {
		width = defaultViewportWidth
	}
	height := m.height
	if height <= 0 {
		height = defaultViewportHeight
	}
	chromeHeight := len(m.headerLines()) + len(m.footerLines()) + 2
	contentHeight := height - chromeHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.viewport.Width = width
	m.viewport.Height = contentHeight
}

func (m StatusModel) renderContent() string {
	switch m.view {
	case viewTasks:
		return m.renderTasks()
	case viewRuns:
		return m.renderRuns()
	case viewRunDetail:
		if m.runDetails != nil {
			return m.renderRunDetails(*m.runDetails)
		}
		return m.renderEmptyRunDetail()
	case viewHelp:
		return m.renderHelp()
	case viewTaskEntry:
		return m.renderTaskEntry()
	default:
		return m.renderDashboard()
	}
}

func (m StatusModel) headerLines() []string {
	state := "not initialized"
	if m.status.Initialized {
		state = "initialized"
	}
	views := "Views: " + m.viewTabs()
	if m.view == viewTaskEntry {
		views = "View: Add Task"
	}
	if m.width > 0 && len(views) > m.width {
		views = "View: " + m.viewLabel()
	}
	return []string{
		"Revolvr",
		views,
		"State: " + state,
	}
}

func (m StatusModel) viewTabs() string {
	labels := []struct {
		view  TUIView
		label string
	}{
		{view: viewDashboard, label: "Dashboard"},
		{view: viewTasks, label: "Tasks"},
		{view: viewRuns, label: "Runs"},
		{view: viewRunDetail, label: "Run Detail"},
		{view: viewHelp, label: "Help"},
	}
	parts := make([]string, 0, len(labels))
	for _, item := range labels {
		if m.view == item.view {
			parts = append(parts, "["+item.label+"]")
			continue
		}
		parts = append(parts, item.label)
	}
	return strings.Join(parts, " | ")
}

func (m StatusModel) viewLabel() string {
	switch m.view {
	case viewTasks:
		return "Tasks"
	case viewRuns:
		return "Runs"
	case viewRunDetail:
		return "Run Detail"
	case viewHelp:
		return "Help"
	case viewTaskEntry:
		return "Add Task"
	default:
		return "Dashboard"
	}
}

func (m StatusModel) footerLines() []string {
	keys := []string{}
	switch m.view {
	case viewTasks:
		keys = append(keys, "j/k Select")
	case viewRuns:
		keys = append(keys, "j/k Select", "enter Open")
	case viewRunDetail:
		keys = append(keys, "enter Reload", "esc Runs")
	case viewHelp:
		keys = append(keys, "esc Back")
	case viewTaskEntry:
		return wrapKeyLines([]string{"tab Field", "enter Submit", "esc Cancel", "ctrl+c Quit"}, m.width)
	}
	keys = append(keys, "1 Dashboard", "2 Tasks", "3 Runs", "4 Detail", "? Help", "a Add Task", "r Refresh", "q Quit")
	return wrapKeyLines(keys, m.width)
}

func (m StatusModel) renderDashboard() string {
	if !m.status.Initialized {
		lines := []string{
			"Dashboard",
			"State: not initialized",
		}
		lines = appendNotice(lines, m.message)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	counts := countTasks(m.status.Tasks)
	lines := []string{
		"Dashboard",
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

func (m StatusModel) renderTasks() string {
	lines := []string{"Tasks"}
	lines = appendNotice(lines, m.message)
	if !m.status.Initialized {
		lines = append(lines, "State: not initialized", "No tasks loaded.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	counts := countTasks(m.status.Tasks)
	lines = append(lines,
		fmt.Sprintf("Total: %d", counts.total),
		fmt.Sprintf("Pending: %d", counts.pending),
		fmt.Sprintf("Blocked: %d", counts.blocked),
		fmt.Sprintf("Completed: %d", counts.completed),
		"",
		"Task List",
	)
	if len(m.status.Tasks) == 0 {
		lines = append(lines,
			"None",
			"",
			"Task Detail",
			"No task selected.",
		)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	selected := clampTaskIndex(m.status.Tasks, m.selectedTask)
	for i, task := range m.status.Tasks {
		prefix := " "
		if i == selected {
			prefix = ">"
		}
		summary := oneLine(task.Summary)
		if summary == "" {
			summary = oneLine(task.Task)
		}
		if summary == "" {
			lines = append(lines, fmt.Sprintf("%s %s  %s", prefix, optionalValue(task.ID), taskListStatus(task.Status)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s  %s  %s", prefix, optionalValue(task.ID), taskListStatus(task.Status), summary))
	}
	lines = append(lines, "")
	lines = append(lines, renderTaskDetailLines(m.status.Tasks[selected])...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderRuns() string {
	lines := []string{"Runs"}
	lines = appendNotice(lines, m.message)
	if !m.status.Initialized {
		lines = append(lines, "State: not initialized", "No runs loaded.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	lines = append(lines, m.recentRunLines()...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderRunDetails(history ledger.RunWithEvents) string {
	run := history.Run
	lines := []string{
		"Run Detail",
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

func (m StatusModel) renderEmptyRunDetail() string {
	lines := []string{
		"Run Detail",
	}
	lines = appendNotice(lines, m.message)
	if !m.status.Initialized {
		lines = append(lines, "State: not initialized", "No run detail loaded.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	lines = append(lines, "No run detail loaded.")
	if len(m.status.RecentRuns) == 0 {
		lines = append(lines, "No runs available.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	lines = append(lines, fmt.Sprintf("Selected run: %s", optionalValue(m.selectedRunID())))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderHelp() string {
	lines := []string{
		"Help",
		"Views",
		"1  Dashboard",
		"2  Tasks",
		"3  Runs",
		"4  Run Detail",
		"?  Help",
		"",
		"Actions",
		"a  Add task",
		"r  Refresh status",
		"q  Quit",
		"",
		"Tasks: j/k Move selection",
		"Runs: j/k Move selection",
		"enter or o  Open selected run",
		"esc  Back from help or run detail",
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderTaskEntry() string {
	lines := []string{
		"Add Task",
		taskEntryLine(m.taskEntry.field == taskEntryTaskField, "Task", m.taskEntry.taskText),
		taskEntryLine(m.taskEntry.field == taskEntrySummaryField, "Summary", m.taskEntry.summary),
	}
	if message := oneLine(m.taskEntry.message); message != "" {
		lines = append(lines, "", "Error: "+message)
	}
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

func renderTaskDetailLines(task taskqueue.Task) []string {
	lines := []string{
		"Task Detail",
		fmt.Sprintf("ID: %s", optionalValue(task.ID)),
		fmt.Sprintf("Status: %s", optionalValue(task.Status)),
		fmt.Sprintf("Summary: %s", optionalValue(task.Summary)),
		fmt.Sprintf("Task: %s", optionalValue(task.Task)),
		fmt.Sprintf("Blocker: %s", optionalValue(task.Blocker)),
	}
	if !task.CreatedAt.IsZero() {
		lines = append(lines, fmt.Sprintf("Created: %s", optionalTime(task.CreatedAt)))
	}
	if !task.UpdatedAt.IsZero() {
		lines = append(lines, fmt.Sprintf("Updated: %s", optionalTime(task.UpdatedAt)))
	}
	if task.BlockedAt != nil {
		lines = append(lines, fmt.Sprintf("Blocked: %s", optionalTimePtr(task.BlockedAt)))
	}
	if task.CompletedAt != nil {
		lines = append(lines, fmt.Sprintf("Completed: %s", optionalTimePtr(task.CompletedAt)))
	}
	return lines
}

func taskListStatus(status string) string {
	switch status {
	case taskqueue.StatusBlocked:
		return "! blocked"
	case "":
		return "none"
	default:
		return status
	}
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

func taskEntryLine(active bool, label string, value string) string {
	prefix := " "
	if active {
		prefix = ">"
	}
	if value == "" {
		return fmt.Sprintf("%s %s:", prefix, label)
	}
	return fmt.Sprintf("%s %s: %s", prefix, label, value)
}

func wrapKeyLines(keys []string, width int) []string {
	const prefix = "Keys: "
	if width <= 0 {
		width = defaultViewportWidth
	}
	lines := []string{prefix}
	for _, key := range keys {
		part := key
		if lines[len(lines)-1] != prefix {
			part = " | " + key
		}
		if len(lines[len(lines)-1])+len(part) > width && lines[len(lines)-1] != prefix {
			lines = append(lines, strings.Repeat(" ", len(prefix))+key)
			continue
		}
		lines[len(lines)-1] += part
	}
	return lines
}

func trimTrailingBlankLines(value string) string {
	lines := strings.Split(value, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (m *StatusModel) moveSelectedTask(delta int) {
	if len(m.status.Tasks) == 0 {
		m.selectedTask = 0
		return
	}
	m.selectedTask = clampTaskIndex(m.status.Tasks, m.selectedTask+delta)
}

func (m StatusModel) selectedTaskID() string {
	if len(m.status.Tasks) == 0 {
		return ""
	}
	return strings.TrimSpace(m.status.Tasks[clampTaskIndex(m.status.Tasks, m.selectedTask)].ID)
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

func selectedTaskIndex(tasks []taskqueue.Task, taskID string) int {
	taskID = strings.TrimSpace(taskID)
	if taskID != "" {
		for i, task := range tasks {
			if strings.TrimSpace(task.ID) == taskID {
				return i
			}
		}
	}
	return clampTaskIndex(tasks, 0)
}

func clampTaskIndex(tasks []taskqueue.Task, index int) int {
	if len(tasks) == 0 || index < 0 {
		return 0
	}
	if index >= len(tasks) {
		return len(tasks) - 1
	}
	return index
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

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}
