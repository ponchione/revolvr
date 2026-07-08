package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"revolvr/internal/app"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
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
	validation   receiptValidationState
	taskEntry    taskEntryState
	message      string
	width        int
	height       int
	viewport     viewport.Model
}

type RefreshStatusFunc func() (app.StatusResult, error)
type OpenRunFunc func(runID string) (ledger.RunWithEvents, error)
type AddTaskFunc func(input app.AddTaskInput) (taskqueue.Task, error)
type ValidateReceiptFunc func(runID string) (receipt.ValidationResult, error)

type StatusActions struct {
	RefreshStatus   RefreshStatusFunc
	OpenRun         OpenRunFunc
	AddTask         AddTaskFunc
	ValidateReceipt ValidateReceiptFunc
}

type RunOptions struct {
	Input           io.Reader
	Output          io.Writer
	RefreshStatus   RefreshStatusFunc
	OpenRun         OpenRunFunc
	AddTask         AddTaskFunc
	ValidateReceipt ValidateReceiptFunc
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

type receiptValidationState struct {
	RunID   string
	Checked bool
	Result  receipt.ValidationResult
	Err     string
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
		RefreshStatus:   opts.RefreshStatus,
		OpenRun:         opts.OpenRun,
		AddTask:         opts.AddTask,
		ValidateReceipt: opts.ValidateReceipt,
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
				m.validation = receiptValidationState{}
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
			m.validation = receiptValidationState{RunID: strings.TrimSpace(msg.history.Run.ID)}
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
				m.validation = receiptValidationState{}
			}
			m.view = viewTasks
			m.taskEntry = taskEntryState{}
			m.message = fmt.Sprintf("Added task %s.", optionalValue(msg.task.ID))
		}
		m.resizeViewport()
		m.updateViewportContent()
		return m, nil
	case validateReceiptMsg:
		m.validation = receiptValidationState{
			RunID:   msg.runID,
			Checked: true,
			Result:  msg.result,
		}
		if msg.err != nil {
			m.validation.Err = msg.err.Error()
			m.message = "Receipt validation error."
		} else if msg.result.Passed() {
			m.message = "Receipt validation passed."
		} else {
			m.message = "Receipt validation failed."
		}
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
			case "v":
				if m.actions.ValidateReceipt == nil {
					m.message = "Receipt validation is unavailable."
					m.updateViewportContent()
					return m, nil
				}
				cmd := m.validateRunReceiptCmd()
				if cmd == nil {
					m.message = "No run detail loaded."
					m.updateViewportContent()
					return m, nil
				}
				return m, cmd
			case "home":
				m.viewport.GotoTop()
				return m, nil
			case "end":
				m.viewport.GotoBottom()
				return m, nil
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

type validateReceiptMsg struct {
	runID  string
	result receipt.ValidationResult
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

func (m StatusModel) validateRunReceiptCmd() tea.Cmd {
	if m.runDetails == nil {
		return nil
	}
	runID := strings.TrimSpace(m.runDetails.Run.ID)
	if runID == "" {
		return nil
	}
	return func() tea.Msg {
		result, err := m.actions.ValidateReceipt(runID)
		return validateReceiptMsg{runID: runID, result: result, err: err}
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
		keys = append(keys, "up/down Scroll", "home/end Jump", "enter Reload", "v Validate", "esc Runs")
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
	diagnostics := runDetailDiagnosticsFromHistory(history)
	lines := []string{"Run Detail"}
	lines = appendNotice(lines, m.message)
	lines = append(lines, runSummaryLines(history.Run)...)
	lines = append(lines, "")
	lines = append(lines, runDiagnosticLines(diagnostics)...)
	lines = append(lines, "")
	lines = append(lines, runReceiptValidationLines(m.validation, history.Run.ID)...)
	lines = append(lines, "")
	lines = append(lines, runChangedFileLines(diagnostics)...)
	lines = append(lines, "")
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
		"Run Detail: up/down Scroll, home/end Jump, v Validate receipt",
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
	lines = append(lines, "ID  STATUS  VERIFICATION  COMMIT  SUMMARY")
	selected := clampRunIndex(m.status.RecentRuns, m.selectedRun)
	for i, run := range m.status.RecentRuns {
		prefix := " "
		if i == selected {
			prefix = ">"
		}
		lines = append(lines, runListLine(prefix, run))
	}
	return lines
}

func runListLine(prefix string, run ledger.Run) string {
	return fmt.Sprintf(
		"%s %s  %s  %s  %s  %s",
		prefix,
		optionalValue(run.ID),
		optionalValue(run.Status),
		optionalValue(run.VerificationStatus),
		optionalValue(run.CommitSHA),
		optionalValue(run.Summary),
	)
}

func runSummaryLines(run ledger.Run) []string {
	return []string{
		"Summary",
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
	}
}

func runArtifactLines(events []ledger.Event) []string {
	lines := []string{"Artifacts"}
	artifacts, found := ledger.RunArtifactsFromEvents(events)
	if !found {
		return append(lines, "No artifact event found.")
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
		path := strings.TrimSpace(artifact.path)
		if path == "" {
			lines = append(lines, fmt.Sprintf("%s: missing", artifact.label))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", artifact.label, path))
	}
	return lines
}

type runDetailDiagnostics struct {
	Outcome                    string
	Message                    string
	CodexSeen                  bool
	CodexExitCode              int
	CodexTimedOut              bool
	CodexError                 string
	VerificationStatus         string
	FailedVerificationCommand  string
	FailedVerificationExitCode *int
	CommitSHA                  string
	CommitStatus               string
	CommitRefusal              string
	CommitMessage              string
	ReceiptVerdict             string
	ReceiptPath                string
	ChangedFiles               []string
	ChangedFilesCaptured       bool
	ChangedFilesCaptureError   string
	Warnings                   []runDetailWarning
	commitSHAFromRun           string
}

type runDetailWarning struct {
	WarningType string
	Message     string
	ReceiptPath string
}

type runDetailVerificationCommand struct {
	Index    int    `json:"index"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	Passed   bool   `json:"passed"`
	ExitCode int    `json:"exit_code"`
}

func runDetailDiagnosticsFromHistory(history ledger.RunWithEvents) runDetailDiagnostics {
	diagnostics := runDetailDiagnostics{commitSHAFromRun: strings.TrimSpace(history.Run.CommitSHA)}
	for _, event := range history.Events {
		switch event.Type {
		case ledger.EventRunCompleted, ledger.EventRunFailed:
			diagnostics.applyRunFinished(event)
		case ledger.EventCodexCompleted:
			diagnostics.applyCodexCompleted(event)
		case ledger.EventVerificationCompleted:
			diagnostics.applyVerificationCompleted(event)
		case ledger.EventCommitCreated:
			diagnostics.applyCommitCreated(event)
		case ledger.EventReceiptParsed, ledger.EventReceiptSynthesized:
			diagnostics.applyReceipt(event)
		case ledger.EventReceiptWarning:
			diagnostics.applyReceiptWarning(event)
		case ledger.EventChangedFilesCaptured:
			diagnostics.applyChangedFiles(event)
		}
	}
	if diagnostics.CommitSHA == "" && diagnostics.usefulWithoutRunCommit() {
		diagnostics.CommitSHA = diagnostics.commitSHAFromRun
	}
	return diagnostics
}

func (d runDetailDiagnostics) empty() bool {
	return !d.usefulWithoutRunCommit() && strings.TrimSpace(d.CommitSHA) == ""
}

func (d runDetailDiagnostics) usefulWithoutRunCommit() bool {
	return strings.TrimSpace(d.Outcome) != "" ||
		strings.TrimSpace(d.Message) != "" ||
		d.CodexSeen ||
		strings.TrimSpace(d.VerificationStatus) != "" ||
		strings.TrimSpace(d.FailedVerificationCommand) != "" ||
		strings.TrimSpace(d.CommitStatus) != "" ||
		strings.TrimSpace(d.CommitRefusal) != "" ||
		strings.TrimSpace(d.CommitMessage) != "" ||
		strings.TrimSpace(d.ReceiptVerdict) != "" ||
		strings.TrimSpace(d.ReceiptPath) != "" ||
		len(d.Warnings) > 0
}

func (d *runDetailDiagnostics) applyRunFinished(event ledger.Event) {
	var payload struct {
		Outcome       string `json:"outcome"`
		Message       string `json:"message"`
		CommitSHA     string `json:"commit_sha"`
		CommitStatus  string `json:"commit_status"`
		CommitRefusal string `json:"commit_refusal"`
		CommitMessage string `json:"commit_message"`
	}
	if !decodeRunDetailPayload(event, &payload) {
		return
	}
	if value := oneLine(payload.Outcome); value != "" {
		d.Outcome = value
	}
	if value := oneLine(payload.Message); value != "" {
		d.Message = value
	}
	if value := oneLine(payload.CommitSHA); value != "" {
		d.CommitSHA = value
	}
	if value := oneLine(payload.CommitStatus); value != "" {
		d.CommitStatus = value
	}
	if value := oneLine(payload.CommitRefusal); value != "" {
		d.CommitRefusal = value
	}
	if value := oneLine(payload.CommitMessage); value != "" {
		d.CommitMessage = value
	}
}

func (d *runDetailDiagnostics) applyCodexCompleted(event ledger.Event) {
	var payload struct {
		ExitCode int    `json:"exit_code"`
		TimedOut bool   `json:"timed_out"`
		Error    string `json:"error"`
	}
	if !decodeRunDetailPayload(event, &payload) {
		return
	}
	d.CodexSeen = true
	d.CodexExitCode = payload.ExitCode
	d.CodexTimedOut = payload.TimedOut
	d.CodexError = oneLine(payload.Error)
}

func (d *runDetailDiagnostics) applyVerificationCompleted(event ledger.Event) {
	var payload struct {
		Status             string                         `json:"status"`
		FailedCommandIndex *int                           `json:"failed_command_index"`
		Commands           []runDetailVerificationCommand `json:"commands"`
	}
	if !decodeRunDetailPayload(event, &payload) {
		return
	}
	if value := oneLine(payload.Status); value != "" {
		d.VerificationStatus = value
	}
	if command, ok := failedRunDetailVerificationCommand(payload.FailedCommandIndex, payload.Commands); ok {
		d.FailedVerificationCommand = command.Command
		exitCode := command.ExitCode
		d.FailedVerificationExitCode = &exitCode
	}
}

func failedRunDetailVerificationCommand(failedIndex *int, commands []runDetailVerificationCommand) (runDetailVerificationCommand, bool) {
	if failedIndex != nil {
		for _, command := range commands {
			if command.Index == *failedIndex && strings.TrimSpace(command.Command) != "" {
				command.Command = oneLine(command.Command)
				return command, true
			}
		}
	}
	for _, command := range commands {
		status := oneLine(command.Status)
		if strings.TrimSpace(command.Command) == "" {
			continue
		}
		if status == "failed" || (status != "" && !command.Passed) {
			command.Command = oneLine(command.Command)
			return command, true
		}
	}
	return runDetailVerificationCommand{}, false
}

func (d *runDetailDiagnostics) applyCommitCreated(event ledger.Event) {
	var payload struct {
		CommitSHA string `json:"commit_sha"`
	}
	if !decodeRunDetailPayload(event, &payload) {
		return
	}
	if value := oneLine(payload.CommitSHA); value != "" {
		d.CommitSHA = value
	}
}

func (d *runDetailDiagnostics) applyReceipt(event ledger.Event) {
	var payload struct {
		ReceiptPath string `json:"receipt_path"`
		Verdict     string `json:"verdict"`
	}
	if !decodeRunDetailPayload(event, &payload) {
		return
	}
	if value := oneLine(payload.ReceiptPath); value != "" {
		d.ReceiptPath = value
	}
	if value := oneLine(payload.Verdict); value != "" {
		d.ReceiptVerdict = value
	}
}

func (d *runDetailDiagnostics) applyReceiptWarning(event ledger.Event) {
	var payload struct {
		WarningType string `json:"warning_type"`
		Message     string `json:"message"`
		ReceiptPath string `json:"receipt_path"`
	}
	if !decodeRunDetailPayload(event, &payload) {
		return
	}
	warning := runDetailWarning{
		WarningType: oneLine(payload.WarningType),
		Message:     oneLine(payload.Message),
		ReceiptPath: oneLine(payload.ReceiptPath),
	}
	if warning.WarningType == "" && warning.Message == "" {
		return
	}
	d.Warnings = append(d.Warnings, warning)
}

func (d *runDetailDiagnostics) applyChangedFiles(event ledger.Event) {
	d.ChangedFilesCaptured = true
	var payload struct {
		ChangedFiles []string `json:"changed_files"`
		CaptureError string   `json:"capture_error"`
	}
	if !decodeRunDetailPayload(event, &payload) {
		d.ChangedFilesCaptureError = "unreadable changed-files event"
		return
	}
	d.ChangedFiles = compactRunDetailStrings(payload.ChangedFiles)
	d.ChangedFilesCaptureError = oneLine(payload.CaptureError)
}

func runDiagnosticLines(diagnostics runDetailDiagnostics) []string {
	lines := []string{"Diagnostics"}
	if diagnostics.empty() {
		return append(lines, "None")
	}
	if value := oneLine(diagnostics.Outcome); value != "" {
		lines = append(lines, "outcome: "+value)
	}
	if value := oneLine(diagnostics.Message); value != "" {
		lines = append(lines, "message: "+value)
	}
	if diagnostics.CodexSeen {
		parts := []string{
			fmt.Sprintf("exit_code=%d", diagnostics.CodexExitCode),
			fmt.Sprintf("timed_out=%t", diagnostics.CodexTimedOut),
		}
		if value := oneLine(diagnostics.CodexError); value != "" {
			parts = append(parts, "error="+value)
		}
		lines = append(lines, "codex: "+strings.Join(parts, ", "))
	}
	if value := oneLine(diagnostics.VerificationStatus); value != "" {
		lines = append(lines, "verification: "+value)
	}
	if value := oneLine(diagnostics.FailedVerificationCommand); value != "" {
		line := "failed verification: " + value
		if diagnostics.FailedVerificationExitCode != nil {
			line += fmt.Sprintf(" (exit_code=%d)", *diagnostics.FailedVerificationExitCode)
		}
		lines = append(lines, line)
	}
	if line := runDetailCommitLine(diagnostics); line != "" {
		lines = append(lines, line)
	}
	if line := runDetailReceiptLine(diagnostics); line != "" {
		lines = append(lines, line)
	}
	for _, warning := range diagnostics.Warnings {
		if line := runDetailWarningLine(warning); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func runReceiptValidationLines(state receiptValidationState, runID string) []string {
	lines := []string{"Receipt Validation"}
	runID = strings.TrimSpace(runID)
	if !state.Checked || strings.TrimSpace(state.RunID) != runID {
		return append(lines, "Status: not run")
	}
	if err := oneLine(state.Err); err != "" {
		lines = append(lines, "Status: error")
		return append(lines, "Error: "+err)
	}
	status := "passed"
	if !state.Result.Passed() {
		status = "failed"
	}
	lines = append(lines,
		"Status: "+status,
		fmt.Sprintf("Run ID: %s", optionalValue(state.Result.RunID)),
		fmt.Sprintf("Receipt: %s", optionalValue(state.Result.ReceiptPath)),
		"Checks:",
	)
	if len(state.Result.Checks) == 0 {
		return append(lines, "No checks returned.")
	}
	for _, check := range state.Result.Checks {
		lines = append(lines, receiptValidationCheckLine(check))
	}
	return lines
}

func receiptValidationCheckLine(check receipt.ValidationCheck) string {
	status := "PASS"
	if !check.Passed {
		status = "FAIL"
	}
	return fmt.Sprintf("%s %s: %s", status, optionalValue(check.Name), oneLine(check.Message()))
}

func runChangedFileLines(diagnostics runDetailDiagnostics) []string {
	lines := []string{"Changed Files"}
	if !diagnostics.ChangedFilesCaptured {
		return append(lines, "No changed-files event found.")
	}
	if value := oneLine(diagnostics.ChangedFilesCaptureError); value != "" {
		lines = append(lines, "Capture error: "+value)
	}
	if len(diagnostics.ChangedFiles) == 0 {
		return append(lines, "None")
	}
	return append(lines, diagnostics.ChangedFiles...)
}

func runDetailCommitLine(d runDetailDiagnostics) string {
	if value := oneLine(d.CommitSHA); value != "" {
		return "commit: " + value
	}
	if value := oneLine(d.CommitRefusal); value != "" {
		line := "commit: refused " + value
		if message := oneLine(d.CommitMessage); message != "" {
			line += ": " + message
		}
		return line
	}
	status := oneLine(d.CommitStatus)
	message := oneLine(d.CommitMessage)
	if status == "" {
		if message == "" {
			return ""
		}
		return "commit: " + message
	}
	if status == "committed" {
		return ""
	}
	line := "commit: " + status
	if message != "" {
		line += ": " + message
	}
	return line
}

func runDetailReceiptLine(d runDetailDiagnostics) string {
	verdict := oneLine(d.ReceiptVerdict)
	path := oneLine(d.ReceiptPath)
	if verdict == "" && path == "" {
		return ""
	}
	if verdict == "" {
		verdict = "recorded"
	}
	if path == "" {
		return "receipt: " + verdict
	}
	return fmt.Sprintf("receipt: %s (%s)", verdict, path)
}

func runDetailWarningLine(warning runDetailWarning) string {
	warningType := oneLine(warning.WarningType)
	message := oneLine(warning.Message)
	if warningType == "" {
		if message == "" {
			return ""
		}
		return "warning: " + message
	}
	line := "warning: " + warningType
	if message != "" {
		line += ": " + message
	}
	if path := oneLine(warning.ReceiptPath); path != "" {
		line += " (" + path + ")"
	}
	return line
}

func decodeRunDetailPayload(event ledger.Event, target any) bool {
	if len(event.Payload) == 0 {
		return false
	}
	return json.Unmarshal(event.Payload, target) == nil
}

func compactRunDetailStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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
