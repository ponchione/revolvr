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
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
)

const (
	defaultViewportWidth  = 80
	defaultViewportHeight = 24
	maxRunLogLines        = 200
	compactLayoutWidth    = 72
)

var _ tea.Model = StatusModel{}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	sectionStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type TUIView int

const (
	viewDashboard TUIView = iota
	viewTasks
	viewRuns
	viewRunDetail
	viewPreflight
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
	runOnce      runOnceState
	preflight    preflightState
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
type PreflightFunc func() (app.PreflightResult, error)
type RunOnceFunc func(context.Context, app.RunProgress) (runonce.Result, error)

type StatusActions struct {
	Context         context.Context
	RefreshStatus   RefreshStatusFunc
	OpenRun         OpenRunFunc
	AddTask         AddTaskFunc
	ValidateReceipt ValidateReceiptFunc
	Preflight       PreflightFunc
	RunOnce         RunOnceFunc
}

type RunOptions struct {
	Input           io.Reader
	Output          io.Writer
	RefreshStatus   RefreshStatusFunc
	OpenRun         OpenRunFunc
	AddTask         AddTaskFunc
	ValidateReceipt ValidateReceiptFunc
	Preflight       PreflightFunc
	RunOnce         RunOnceFunc
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

type preflightState struct {
	Checked bool
	Result  app.PreflightResult
	Err     string
}

type runOnceState struct {
	Active          bool
	Started         bool
	CancelRequested bool
	Token           int
	Cancel          context.CancelFunc
	Messages        <-chan tea.Msg
	Status          string
	RunID           string
	Outcome         string
	Err             string
	Logs            []string
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
		Context:         ctx,
		RefreshStatus:   opts.RefreshStatus,
		OpenRun:         opts.OpenRun,
		AddTask:         opts.AddTask,
		ValidateReceipt: opts.ValidateReceipt,
		Preflight:       opts.Preflight,
		RunOnce:         opts.RunOnce,
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
	case preflightMsg:
		m.preflight = preflightState{
			Checked: true,
			Result:  msg.result,
		}
		if msg.err != nil {
			m.preflight.Err = msg.err.Error()
			m.message = "Preflight error."
		} else if msg.result.Ready {
			m.message = "Preflight ready."
		} else {
			m.message = "Preflight failed."
		}
		m.updateViewportContent()
		return m, nil
	case runOnceProgressMsg:
		if msg.token != m.runOnce.Token || !m.runOnce.Started {
			return m, nil
		}
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, runProgressLine(msg.event))
		m.message = "Run in progress."
		m.updateViewportContent()
		return m, m.waitRunOnceMsgCmd()
	case runOnceDoneMsg:
		if msg.token != m.runOnce.Token || !m.runOnce.Started {
			return m, nil
		}
		m.applyRunOnceDone(msg)
		m.updateViewportContent()
		return m, nil
	case tea.KeyMsg:
		if m.view == viewTaskEntry {
			return m.updateTaskEntry(msg)
		}
		if handled, cmd := m.updateActiveRunKeys(msg); handled {
			return m, cmd
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
		case "5":
			m.switchView(viewPreflight)
			if m.runOnce.Active {
				return m, nil
			}
			if m.actions.Preflight == nil {
				m.message = "Preflight is unavailable."
				m.updateViewportContent()
				return m, nil
			}
			return m, m.preflightCmd()
		case "?":
			m.switchView(viewHelp)
			return m, nil
		case "a":
			m.startTaskEntry()
			return m, nil
		case "R":
			cmd := m.startRunOnce()
			return m, cmd
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
		case viewPreflight:
			switch msg.String() {
			case "p":
				if m.actions.Preflight == nil {
					m.message = "Preflight is unavailable."
					m.updateViewportContent()
					return m, nil
				}
				return m, m.preflightCmd()
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m StatusModel) View() string {
	sections := append([]string{}, styleHeaderLines(m.headerDisplayLines())...)
	sections = append(sections, "")
	sections = append(sections, trimTrailingBlankLines(m.viewport.View()))
	sections = append(sections, "")
	sections = append(sections, styleFooterLines(m.footerLines())...)
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

type preflightMsg struct {
	result app.PreflightResult
	err    error
}

type runOnceProgressMsg struct {
	token int
	event codexexec.ProgressEvent
}

type runOnceDoneMsg struct {
	token         int
	result        runonce.Result
	err           error
	cancelled     bool
	status        app.StatusResult
	statusErr     error
	history       ledger.RunWithEvents
	historyLoaded bool
	historyErr    error
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

func (m StatusModel) preflightCmd() tea.Cmd {
	return func() tea.Msg {
		result, err := m.actions.Preflight()
		return preflightMsg{result: result, err: err}
	}
}

func (m *StatusModel) startRunOnce() tea.Cmd {
	if message := m.runStartBlocker(); message != "" {
		m.message = message
		m.updateViewportContent()
		return nil
	}

	baseCtx := m.actions.Context
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	runCtx, cancel := context.WithCancel(baseCtx)
	token := m.runOnce.Token + 1
	messages := make(chan tea.Msg, 128)
	m.runOnce = runOnceState{
		Active:   true,
		Started:  true,
		Token:    token,
		Cancel:   cancel,
		Messages: messages,
		Status:   "running",
		Logs:     []string{"system: run started"},
	}
	m.message = "Run started."
	m.updateViewportContent()
	return m.startRunOnceCmd(token, runCtx, messages)
}

func (m StatusModel) startRunOnceCmd(token int, ctx context.Context, messages chan tea.Msg) tea.Cmd {
	actions := m.actions
	return func() tea.Msg {
		go func() {
			result, err := actions.RunOnce(ctx, func(event codexexec.ProgressEvent) {
				line := runProgressLine(event)
				if line == "" {
					return
				}
				select {
				case messages <- runOnceProgressMsg{token: token, event: event}:
				case <-ctx.Done():
				}
			})

			done := runOnceDoneMsg{
				token:     token,
				result:    result,
				err:       err,
				cancelled: ctx.Err() != nil,
			}
			if actions.RefreshStatus != nil {
				done.status, done.statusErr = actions.RefreshStatus()
			} else {
				done.statusErr = fmt.Errorf("refresh is unavailable")
			}
			runID := strings.TrimSpace(result.Run.ID)
			if runID != "" {
				if actions.OpenRun != nil {
					done.history, done.historyErr = actions.OpenRun(runID)
					done.historyLoaded = done.historyErr == nil
				} else {
					done.historyErr = fmt.Errorf("open run is unavailable")
				}
			}
			messages <- done
			close(messages)
		}()

		msg, ok := <-messages
		if !ok {
			return runOnceDoneMsg{
				token:     token,
				err:       fmt.Errorf("run event stream closed"),
				cancelled: ctx.Err() != nil,
			}
		}
		return msg
	}
}

func (m StatusModel) waitRunOnceMsgCmd() tea.Cmd {
	messages := m.runOnce.Messages
	token := m.runOnce.Token
	if messages == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-messages
		if !ok {
			return runOnceDoneMsg{token: token, err: fmt.Errorf("run event stream closed")}
		}
		return msg
	}
}

func (m StatusModel) runStartBlocker() string {
	switch {
	case m.runOnce.Active:
		return "Run already active."
	case m.actions.RunOnce == nil:
		return "Run is unavailable."
	case !m.preflight.Checked:
		return "Run blocked: preflight is not ready."
	case strings.TrimSpace(m.preflight.Err) != "":
		return "Run blocked: preflight error: " + oneLine(m.preflight.Err)
	case !m.preflight.Result.Ready:
		return "Run blocked: preflight is not ready."
	default:
		return ""
	}
}

func (m *StatusModel) updateActiveRunKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.runOnce.Active {
		return false, nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		m.requestRunCancel()
		return true, tea.Quit
	case "c":
		m.requestRunCancel()
		return true, nil
	case "R", "r", "a":
		m.message = "Run is active; cancel or wait before starting another action."
		m.updateViewportContent()
		return true, nil
	case "p":
		if m.view == viewPreflight {
			m.message = "Run is active; cancel or wait before starting another action."
			m.updateViewportContent()
			return true, nil
		}
	case "v":
		if m.view == viewRunDetail {
			m.message = "Run is active; cancel or wait before starting another action."
			m.updateViewportContent()
			return true, nil
		}
	case "enter", "o":
		if m.view == viewRuns || m.view == viewRunDetail {
			m.message = "Run is active; cancel or wait before starting another action."
			m.updateViewportContent()
			return true, nil
		}
	}
	return false, nil
}

func (m *StatusModel) requestRunCancel() {
	if m.runOnce.CancelRequested {
		m.message = "Cancellation already requested."
		m.updateViewportContent()
		return
	}
	m.runOnce.CancelRequested = true
	if m.runOnce.Cancel != nil {
		m.runOnce.Cancel()
	}
	m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: cancellation requested")
	m.message = "Cancellation requested."
	m.updateViewportContent()
}

func (m *StatusModel) applyRunOnceDone(msg runOnceDoneMsg) {
	runID := strings.TrimSpace(msg.result.Run.ID)
	outcome := strings.TrimSpace(string(msg.result.Outcome))
	terminal := runTerminalStatus(msg.result, msg.err, msg.cancelled)

	m.runOnce.Active = false
	m.runOnce.Cancel = nil
	m.runOnce.Messages = nil
	m.runOnce.Status = terminal
	m.runOnce.RunID = runID
	m.runOnce.Outcome = outcome
	m.runOnce.Err = ""
	if msg.err != nil {
		m.runOnce.Err = msg.err.Error()
	}
	m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: terminal state: "+terminal)

	switch terminal {
	case "cancelled":
		m.message = "Run cancelled."
	case "completed":
		m.message = "Run completed."
	case "no_task":
		m.message = "Run finished: no pending runnable tasks."
	default:
		m.message = "Run failed."
	}
	if runID != "" {
		m.message += " " + runID + "."
	}

	m.applyRunCompletionStatus(msg, runID)
	m.applyRunCompletionDetail(msg, runID)
}

func (m *StatusModel) applyRunCompletionStatus(msg runOnceDoneMsg, runID string) {
	if msg.statusErr != nil {
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: refresh failed: "+oneLine(msg.statusErr.Error()))
		return
	}
	selectedTaskID := m.selectedTaskID()
	selectedRunID := m.selectedRunID()
	if runID != "" {
		selectedRunID = runID
	}
	m.status = msg.status
	m.selectedTask = selectedTaskIndex(m.status.Tasks, selectedTaskID)
	m.selectedRun = selectedRunIndex(m.status.RecentRuns, selectedRunID)
	if !m.status.Initialized {
		m.runDetails = nil
		m.validation = receiptValidationState{}
	}
}

func (m *StatusModel) applyRunCompletionDetail(msg runOnceDoneMsg, runID string) {
	if runID == "" {
		return
	}
	if msg.historyLoaded {
		m.runDetails = &msg.history
		m.validation = receiptValidationState{RunID: runID}
		return
	}
	m.runDetails = nil
	if msg.historyErr != nil {
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: run detail refresh failed: "+oneLine(msg.historyErr.Error()))
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
	m.viewport.SetContent(m.formatContent(m.renderContent()))
	m.viewport.GotoTop()
}

func (m *StatusModel) refreshViewportContent() {
	m.viewport.SetContent(m.formatContent(m.renderContent()))
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
	chromeHeight := len(m.headerDisplayLines()) + len(m.footerLines()) + 2
	contentHeight := height - chromeHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.viewport.Width = width
	m.viewport.Height = contentHeight
}

func (m StatusModel) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = defaultViewportWidth
	}
	if width < 1 {
		return 1
	}
	return width
}

func (m StatusModel) compactLayout() bool {
	return m.contentWidth() < compactLayoutWidth
}

func (m StatusModel) formatContent(content string) string {
	lines := wrapPlainLines(strings.Split(content, "\n"), m.contentWidth())
	for i, line := range lines {
		lines[i] = styleContentLine(line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderContent() string {
	var content string
	switch m.view {
	case viewTasks:
		content = m.renderTasks()
	case viewRuns:
		content = m.renderRuns()
	case viewRunDetail:
		if m.runDetails != nil {
			content = m.renderRunDetails(*m.runDetails)
			break
		}
		content = m.renderEmptyRunDetail()
	case viewPreflight:
		content = m.renderPreflight()
	case viewHelp:
		content = m.renderHelp()
	case viewTaskEntry:
		content = m.renderTaskEntry()
	default:
		content = m.renderDashboard()
	}
	if m.runOnce.Started && m.view != viewHelp && m.view != viewTaskEntry {
		return lipgloss.JoinVertical(lipgloss.Left, m.renderRunProgress(), "", content)
	}
	return content
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

func (m StatusModel) headerDisplayLines() []string {
	return wrapPlainLines(m.headerLines(), m.contentWidth())
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
		{view: viewPreflight, label: "Preflight"},
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
	case viewPreflight:
		return "Preflight"
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
	if m.runOnce.Active {
		switch m.view {
		case viewTasks, viewRuns:
			keys = append(keys, "j/k Select")
		case viewRunDetail:
			keys = append(keys, "up/down Scroll", "home/end Jump")
		case viewHelp:
			keys = append(keys, "esc Back")
		case viewTaskEntry:
			return wrapKeyLines([]string{"tab Field", "enter Submit", "esc Cancel", "ctrl+c Quit"}, m.width)
		}
		keys = append(keys, "1 Dashboard", "2 Tasks", "3 Runs", "4 Detail", "5 Preflight", "? Help", "c Cancel Run", "q Quit")
		return wrapKeyLines(keys, m.width)
	}
	switch m.view {
	case viewTasks:
		keys = append(keys, "j/k Select")
	case viewRuns:
		keys = append(keys, "j/k Select", "enter Open")
	case viewRunDetail:
		keys = append(keys, "up/down Scroll", "home/end Jump", "enter Reload", "v Validate", "esc Runs")
	case viewPreflight:
		keys = append(keys, "p Check")
	case viewHelp:
		keys = append(keys, "esc Back")
	case viewTaskEntry:
		return wrapKeyLines([]string{"tab Field", "enter Submit", "esc Cancel", "ctrl+c Quit"}, m.width)
	}
	keys = append(keys, "1 Dashboard", "2 Tasks", "3 Runs", "4 Detail", "5 Preflight", "? Help", "a Add Task", "R Run Once", "r Refresh", "q Quit")
	return wrapKeyLines(keys, m.width)
}

func (m StatusModel) renderDashboard() string {
	if !m.status.Initialized {
		lines := []string{
			"Dashboard",
			"State: not initialized",
			"Tasks: unavailable",
			"Runs: unavailable",
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
		lines = append(lines, "State: not initialized", "Task List", "Unavailable until state is initialized.")
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
			"No tasks queued.",
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
		lines = append(lines, "State: not initialized", "Recent Runs", "Unavailable until state is initialized.")
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
		lines = append(lines, "No runs recorded.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	lines = append(lines, fmt.Sprintf("Selected run: %s", optionalValue(m.selectedRunID())))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderPreflight() string {
	lines := []string{"Preflight"}
	lines = appendNotice(lines, m.message)
	if !m.preflight.Checked {
		lines = append(lines, "Status: not run", "No readiness result loaded.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	if err := oneLine(m.preflight.Err); err != "" {
		lines = append(lines, "Status: error", "Error: "+err)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	status := "failed"
	if m.preflight.Result.Ready {
		status = "ready"
	}
	lines = append(lines,
		"Status: "+status,
		fmt.Sprintf("Ready: %t", m.preflight.Result.Ready),
		"Checks",
	)
	if len(m.preflight.Result.Checks) == 0 {
		lines = append(lines, "No checks returned.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	for _, check := range m.preflight.Result.Checks {
		lines = append(lines, preflightCheckLine(check))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderRunProgress() string {
	lines := []string{"Run Progress"}
	lines = appendNotice(lines, m.message)
	lines = append(lines, "Status: "+optionalValue(m.runOnce.Status))
	if m.runOnce.RunID != "" {
		lines = append(lines, "Run ID: "+m.runOnce.RunID)
	}
	if m.runOnce.Outcome != "" {
		lines = append(lines, "Outcome: "+m.runOnce.Outcome)
	}
	if m.runOnce.CancelRequested && m.runOnce.Active {
		lines = append(lines, "Cancellation: requested")
	}
	if m.runOnce.Err != "" {
		lines = append(lines, "Error: "+oneLine(m.runOnce.Err))
	}
	lines = append(lines, "Log")
	if len(m.runOnce.Logs) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No progress yet.")...)
	}
	lines = append(lines, m.runOnce.Logs...)
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
		"5  Preflight",
		"?  Help",
		"a  Add task",
		"R  Run once",
		"r  Refresh status",
		"c  Cancel active run",
		"q  Quit",
		"",
		"Tasks: j/k Move selection",
		"Runs: j/k Move selection",
		"Run Detail: up/down Scroll, home/end Jump, v Validate receipt",
		"Preflight: p Run readiness checks",
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
		return append(lines, "No runs recorded.")
	}
	if m.compactLayout() {
		lines = append(lines, "ID  STATUS  SUMMARY")
	} else {
		lines = append(lines, "ID  STATUS  VERIFICATION  COMMIT  SUMMARY")
	}
	selected := clampRunIndex(m.status.RecentRuns, m.selectedRun)
	for i, run := range m.status.RecentRuns {
		prefix := " "
		if i == selected {
			prefix = ">"
		}
		if m.compactLayout() {
			lines = append(lines, runCompactListLine(prefix, run))
		} else {
			lines = append(lines, runListLine(prefix, run))
		}
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

func runCompactListLine(prefix string, run ledger.Run) string {
	return fmt.Sprintf(
		"%s %s  %s  %s",
		prefix,
		optionalValue(run.ID),
		optionalValue(run.Status),
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

func preflightCheckLine(check app.PreflightCheck) string {
	return fmt.Sprintf("%s %s: %s", optionalValue(string(check.Status)), optionalValue(check.Name), oneLine(check.Detail))
}

func runProgressLine(event codexexec.ProgressEvent) string {
	source := oneLine(event.Source)
	message := oneLine(event.Message)
	if message == "" {
		return ""
	}
	if source == "" {
		source = "codex"
	}
	return source + ": " + message
}

func runTerminalStatus(result runonce.Result, err error, cancelled bool) string {
	switch {
	case cancelled:
		return "cancelled"
	case err != nil:
		return "failed"
	case result.NoTask || result.Outcome == runonce.OutcomeNoTask:
		return "no_task"
	case result.Outcome == runonce.OutcomeCommitted:
		return "completed"
	case app.RunOnceOutcomeError(result) != nil:
		return "failed"
	default:
		return "completed"
	}
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

func appendRunLog(logs []string, line string) []string {
	line = oneLine(line)
	if line == "" {
		return logs
	}
	logs = append(logs, line)
	if len(logs) <= maxRunLogLines {
		return logs
	}
	return append([]string(nil), logs[len(logs)-maxRunLogLines:]...)
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
	return wrapPlainLines(lines, width)
}

func wrapPlainLines(lines []string, width int) []string {
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapPlainLine(line, width)...)
	}
	return wrapped
}

func wrapPlainLine(line string, width int) []string {
	if width <= 0 {
		width = defaultViewportWidth
	}
	if width < 1 {
		width = 1
	}
	if textWidth(line) <= width {
		return []string{line}
	}
	if strings.TrimSpace(line) == "" {
		return []string{""}
	}

	indent := leadingWhitespace(line)
	if textWidth(indent) >= width {
		indent = ""
	}
	continuation := indent + "  "
	if textWidth(continuation) >= width {
		continuation = indent
	}
	if textWidth(continuation) >= width {
		continuation = ""
	}

	words := strings.Fields(strings.TrimSpace(line))
	out := make([]string, 0, 2)
	current := indent
	for _, word := range words {
		candidate := current
		if strings.TrimSpace(candidate) != "" {
			candidate += " "
		}
		candidate += word
		if textWidth(candidate) <= width {
			current = candidate
			continue
		}

		if strings.TrimSpace(current) != "" {
			out = append(out, current)
			current = continuation
		}

		for word != "" && textWidth(current)+textWidth(word) > width {
			available := width - textWidth(current)
			if available <= 0 {
				if strings.TrimSpace(current) != "" {
					out = append(out, current)
				}
				current = ""
				available = width
			}
			part, rest := splitRunePrefix(word, available)
			if part == "" {
				break
			}
			out = append(out, current+part)
			word = rest
			current = continuation
		}
		if word != "" {
			current += word
		}
	}
	if strings.TrimSpace(current) != "" {
		out = append(out, current)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func styleHeaderLines(lines []string) []string {
	styled := append([]string(nil), lines...)
	if len(styled) == 0 {
		return styled
	}
	styled[0] = titleStyle.Render(styled[0])
	for i := 1; i < len(styled); i++ {
		styled[i] = mutedStyle.Render(styled[i])
	}
	return styled
}

func styleFooterLines(lines []string) []string {
	styled := append([]string(nil), lines...)
	for i, line := range styled {
		styled[i] = mutedStyle.Render(line)
	}
	return styled
}

func styleContentLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return line
	}
	if isSectionHeading(trimmed) {
		return sectionStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "Notice:") {
		return warningStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "FAIL ") ||
		strings.HasPrefix(trimmed, "Error:") ||
		strings.Contains(trimmed, "! blocked") ||
		lineStatusIn(trimmed, "failed", "error", "blocked", "cancelled") {
		return dangerStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "OK ") ||
		strings.HasPrefix(trimmed, "PASS ") ||
		lineStatusIn(trimmed, "ready", "passed", "completed", "initialized") {
		return successStyle.Render(line)
	}
	if lineStatusIn(trimmed, "not run", "running", "not initialized") ||
		strings.HasPrefix(trimmed, "Cancellation:") ||
		strings.HasPrefix(trimmed, "Capture error:") {
		return warningStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, ">") {
		return selectedStyle.Render(line)
	}
	return line
}

func isSectionHeading(value string) bool {
	switch value {
	case "Dashboard",
		"Tasks",
		"Task List",
		"Task Detail",
		"Latest Run",
		"Recent Runs",
		"Runs",
		"Run Detail",
		"Summary",
		"Diagnostics",
		"Receipt Validation",
		"Changed Files",
		"Artifacts",
		"Events",
		"Preflight",
		"Run Progress",
		"Log",
		"Help",
		"Views",
		"Add Task",
		"Checks":
		return true
	default:
		return false
	}
}

func lineStatusIn(line string, statuses ...string) bool {
	for _, prefix := range []string{"Status: ", "State: "} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		for _, status := range statuses {
			if value == status {
				return true
			}
		}
	}
	return false
}

func leadingWhitespace(value string) string {
	for i, r := range value {
		if r != ' ' && r != '\t' {
			return value[:i]
		}
	}
	return value
}

func splitRunePrefix(value string, n int) (string, string) {
	if n <= 0 {
		return "", value
	}
	runes := []rune(value)
	if n >= len(runes) {
		return value, ""
	}
	return string(runes[:n]), string(runes[n:])
}

func textWidth(value string) int {
	return len([]rune(value))
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
