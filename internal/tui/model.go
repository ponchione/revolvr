package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"revolvr/internal/app"
	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousview"
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/taskscheduler"
)

const (
	defaultViewportWidth  = 80
	defaultViewportHeight = 24
	maxRunLogLines        = 200
	compactLayoutWidth    = 72
	defaultRunLoopPasses  = 3
	maxDashboardEvents    = 8
	maxDashboardDetail    = 56
	maxCommandRows        = 5
)

var _ tea.Model = StatusModel{}

var runLoopPassOptions = []int{2, 3, 5}

var (
	sectionStyle  = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warningStyle  = lipgloss.NewStyle().Bold(true)
	dangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	mutedStyle    = lipgloss.NewStyle().Faint(true)
)

type TUIView int

const (
	viewDashboard TUIView = iota
	viewTasks
	viewRuns
	viewRunDetail
	viewPreflight
	viewAutonomous
	viewHelp
	viewTaskEntry
	viewDiff
	viewEvidence
	viewApproval
)

const (
	runModeOnce  = "once"
	runModeLoop  = "loop"
	runModeTask  = "task"
	runModeQueue = "queue"
)

type StatusModel struct {
	status                app.StatusResult
	actions               StatusActions
	committed             []transcriptCell
	emitted               map[string]struct{}
	settling              *transcriptSettlement
	view                  TUIView
	previous              TUIView
	selectedTask          int
	selectedRun           int
	loopPasses            int
	runDetails            *ledger.RunWithEvents
	runOnce               runOnceState
	preflight             preflightState
	autonomous            autonomousState
	validation            receiptValidationState
	taskEntry             taskEntryState
	composer              commandComposerState
	overlay               *overlayState
	overlayOwner          int
	focusSource           TUIView
	focusedFromAutonomous bool
	message               string
	width                 int
	height                int
	viewport              viewport.Model
}

type transcriptCell struct {
	kind     transcriptCellKind
	identity string
	source   []string
}

type transcriptSettlement struct {
	token int
	cell  transcriptCell
}

type transcriptCellKind string

const (
	transcriptCellSession        transcriptCellKind = "session"
	transcriptCellOperatorAction transcriptCellKind = "operator-action"
	transcriptCellStatus         transcriptCellKind = "status"
	transcriptCellProgress       transcriptCellKind = "progress"
	transcriptCellResult         transcriptCellKind = "result"
	transcriptCellWarning        transcriptCellKind = "warning"
	transcriptCellQuestion       transcriptCellKind = "question"
)

func newSessionTranscriptCell(projectRoot string, initialized bool) transcriptCell {
	initialState := "not initialized"
	if initialized {
		initialState = "initialized"
	}
	return transcriptCell{
		kind:     transcriptCellSession,
		identity: "session-start",
		source: []string{
			"Revolvr",
			"Project: " + projectRoot,
			"At start: " + initialState,
		},
	}
}

func historicalTranscriptCells(status app.StatusResult) []transcriptCell {
	if !status.Initialized || len(status.RecentRuns) == 0 {
		return nil
	}

	history := ledger.RunWithEvents{Run: status.RecentRuns[0], Events: status.LatestEvents}
	type timelineRow struct {
		index int
		row   app.RunTimelineRow
	}
	visible := make([]timelineRow, 0, len(history.Events))
	for index, row := range app.RunTimeline(history) {
		if row.Phase == "run" || row.Phase == "task" ||
			row.Phase == "codex" && row.Status == "progress" && !strings.HasPrefix(row.Detail, "message: ") ||
			row.Phase == "verification" && row.Detail == "from run record" ||
			row.Phase == "commit" && row.Status == "created" && strings.TrimPrefix(strings.TrimSpace(row.Detail), "commit ") == strings.TrimSpace(history.Run.CommitSHA) {
			continue
		}
		visible = append(visible, timelineRow{index: index, row: row})
	}

	start := max(len(visible)-maxDashboardEvents, 0)
	cells := make([]transcriptCell, 0, len(visible)-start+2)
	if start > 0 {
		cells = append(cells, transcriptCell{
			kind:     transcriptCellStatus,
			identity: fmt.Sprintf("run:%s:window:%d", history.Run.ID, visible[start].index),
			source:   []string{fmt.Sprintf("… %d earlier · 4 Run Detail", start)},
		})
	}
	for _, item := range visible[start:] {
		kind := transcriptCellStatus
		switch item.row.Status {
		case "started", "selected", "progress":
			kind = transcriptCellProgress
		case "warning", "failed", "error", "cancelled":
			kind = transcriptCellWarning
		}
		cells = append(cells, transcriptCell{
			kind:     kind,
			identity: fmt.Sprintf("run:%s:timeline:%d:%s:%s", history.Run.ID, item.index, item.row.Phase, item.row.Status),
			source:   []string{dashboardTimelineLine(item.row)},
		})
	}
	if cell, ok := historicalRunResultCell(history.Run); ok {
		cells = append(cells, cell)
	}
	return cells
}

func historicalRunResultCell(run ledger.Run) (transcriptCell, bool) {
	runID := strings.TrimSpace(run.ID)
	status := strings.TrimSpace(run.Status)
	if runID == "" || status == "" {
		return transcriptCell{}, false
	}
	name := oneLine(run.Task)
	if name == "" {
		name = strings.TrimSpace(run.TaskID)
	}
	if name == "" {
		name = runID
	}

	cell := transcriptCell{identity: "run:" + runID + ":status:" + status}
	switch status {
	case ledger.StatusCompleted:
		cell.kind = transcriptCellResult
		cell.source = []string{"Completed: " + name}
		if verification := oneLine(run.VerificationStatus); verification != "" {
			cell.source = append(cell.source, "Verification: "+verification)
		}
		if commit := strings.TrimSpace(run.CommitSHA); commit != "" {
			if len(commit) > 12 {
				commit = commit[:12]
			}
			cell.source = append(cell.source, "Commit: "+commit)
		}
		cell.source = append(cell.source, "Next: /run to continue")
	case ledger.StatusFailed:
		cell.kind = transcriptCellWarning
		cell.source = []string{"Failed: " + name}
		reason := oneLine(run.Summary)
		if run.VerificationStatus == "failed" {
			reason = "verification failed"
		}
		if reason == "" {
			reason = "run failed"
		}
		cell.source = append(cell.source, "Reason: "+reason)
		if detail := oneLine(run.Summary); detail != "" && detail != reason {
			cell.source = append(cell.source, "Detail: "+detail)
		}
		cell.source = append(cell.source, "Next: /detail to inspect the failure")
	default:
		cell.kind = transcriptCellStatus
		cell.source = []string{"Run " + status + ": " + name, "Next: /detail to inspect the run"}
	}
	return cell, true
}

func (c transcriptCell) render(width int) []string {
	source := make([]string, 0, len(c.source))
	for _, line := range c.source {
		line = strings.ReplaceAll(strings.ReplaceAll(line, "\r\n", "\n"), "\r", "\n")
		source = append(source, strings.Split(line, "\n")...)
	}
	if !c.valid() {
		generic := []string{fmt.Sprintf("Warning: unrecognized transcript evidence (%q, %q).", c.kind, c.identity)}
		for _, line := range source {
			generic = append(generic, "Evidence: "+line)
		}
		source = generic
	}
	lines := wrapPlainLines(source, width)
	for i := range lines {
		lines[i] = styleContentLine(lines[i])
	}
	return lines
}

func (c transcriptCell) valid() bool {
	if strings.TrimSpace(c.identity) == "" || len(c.source) == 0 {
		return false
	}
	for _, line := range c.source {
		if strings.TrimSpace(line) == "" || strings.ContainsAny(line, "\r\n") {
			return false
		}
	}
	switch c.kind {
	case transcriptCellSession:
		return c.identity == "session-start" &&
			len(c.source) == 3 &&
			c.source[0] == "Revolvr" &&
			strings.HasPrefix(c.source[1], "Project: ") &&
			(c.source[2] == "At start: initialized" || c.source[2] == "At start: not initialized")
	case transcriptCellOperatorAction,
		transcriptCellStatus,
		transcriptCellProgress,
		transcriptCellResult,
		transcriptCellWarning,
		transcriptCellQuestion:
		return true
	default:
		return false
	}
}

type RefreshStatusFunc func() (app.StatusResult, error)
type OpenRunFunc func(runID string) (ledger.RunWithEvents, error)
type AddTaskFunc func(input app.AddTaskInput) (taskmodel.Task, error)
type RetryTaskFunc func(taskID string) (taskmodel.Task, error)
type ValidateReceiptFunc func(runID string) (receipt.ValidationResult, error)
type PreflightFunc func() (app.PreflightResult, error)
type RunOnceFunc func(context.Context, app.RunProgress) (runonce.Result, error)
type RunLoopFunc func(context.Context, int, app.RunProgress, app.RunPassFunc) (app.RunLoopResult, error)
type RunTaskFunc func(context.Context, string, int64, autonomoustaskrun.Progress) (autonomoustaskrun.Result, error)
type ListAutonomousFunc func() ([]app.AutonomousTaskSelector, error)
type LoadAutonomousFunc func(string) (autonomousview.View, error)
type AnswerAutonomousFunc func(app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error)
type RunQueueFunc func(context.Context, int64, int64, autonomousqueue.Progress) (autonomousqueue.Result, error)

type StatusActions struct {
	Context         context.Context
	RefreshStatus   RefreshStatusFunc
	OpenRun         OpenRunFunc
	AddTask         AddTaskFunc
	RetryTask       RetryTaskFunc
	ValidateReceipt ValidateReceiptFunc
	Preflight       PreflightFunc
	RunOnce         RunOnceFunc
	RunLoop         RunLoopFunc
	RunTask         RunTaskFunc
	ListAutonomous  ListAutonomousFunc
	LoadAutonomous  LoadAutonomousFunc
	AnswerInput     AnswerAutonomousFunc
	RunQueue        RunQueueFunc
}

type RunOptions struct {
	Input           io.Reader
	Output          io.Writer
	RefreshStatus   RefreshStatusFunc
	OpenRun         OpenRunFunc
	AddTask         AddTaskFunc
	RetryTask       RetryTaskFunc
	ValidateReceipt ValidateReceiptFunc
	Preflight       PreflightFunc
	RunOnce         RunOnceFunc
	RunLoop         RunLoopFunc
	RunTask         RunTaskFunc
	ListAutonomous  ListAutonomousFunc
	LoadAutonomous  LoadAutonomousFunc
	AnswerInput     AnswerAutonomousFunc
	RunQueue        RunQueueFunc
}

type autonomousAnswerState struct {
	Active     bool
	Selected   int
	Confirming bool
	Submitting bool
	Snapshot   autonomousAnswerSnapshot
	Result     app.AnswerAutonomousInputResult
	Err        string
}

type autonomousAnswerSnapshot struct {
	TaskID     string
	QuestionID string
	Revision   int64
	ContentSHA string
	OptionID   string
}

type autonomousState struct {
	Selectors   []app.AutonomousTaskSelector
	Selected    int
	Selector    string
	TaskID      string
	Request     int
	LoadingList bool
	LoadingView bool
	View        *autonomousview.View
	Err         string
	Answer      autonomousAnswerState
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

type commandComposerState struct {
	Active          bool
	Text            string
	DiscoveryOpen   bool
	SelectedCommand int
}

type overlayState struct {
	content      TUIView
	source       TUIView
	composer     commandComposerState
	selected     int
	message      string
	sourceOffset int
	parentOffset int
	owner        int
	viewport     viewport.Model
}

type slashCommand struct {
	name        string
	usage       string
	description string
}

var slashCommands = []slashCommand{
	{name: "help", usage: "/help", description: "Open full Help"},
	{name: "commands", usage: "/commands", description: "Alias for /help"},
	{name: "transcript", usage: "/transcript", description: "Open transcript"},
	{name: "dashboard", usage: "/dashboard", description: "Alias for /transcript"},
	{name: "tasks", usage: "/tasks", description: "Open Tasks"},
	{name: "runs", usage: "/runs", description: "Open Runs"},
	{name: "detail", usage: "/detail", description: "Open Run Detail"},
	{name: "workflow", usage: "/workflow", description: "Open Workflow"},
	{name: "diff", usage: "/diff", description: "Open Change Summary"},
	{name: "evidence", usage: "/evidence", description: "Open Evidence"},
	{name: "approval", usage: "/approval", description: "Open Approval"},
	{name: "answer", usage: "/answer <option-id>", description: "Answer typed input"},
	{name: "preflight", usage: "/preflight", description: "Run readiness checks"},
	{name: "run", usage: "/run", description: "Run once"},
	{name: "loop", usage: "/loop", description: "Run bounded loop"},
	{name: "task-run", usage: "/task-run", description: "Run selected task"},
	{name: "queue", usage: "/queue", description: "Run task queue"},
	{name: "refresh", usage: "/refresh", description: "Refresh status"},
	{name: "cancel", usage: "/cancel", description: "Cancel active run"},
	{name: "validate", usage: "/validate", description: "Validate receipt"},
	{name: "quit", usage: "/quit", description: "Quit"},
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
	Active              bool
	Started             bool
	CancelRequested     bool
	QuitAfterSettlement bool
	Mode                string
	Token               int
	Cancel              context.CancelFunc
	Messages            <-chan tea.Msg
	Task                string
	StartedAt           time.Time
	Current             string
	Status              string
	RunID               string
	Outcome             string
	MaxPasses           int
	Stats               app.RunLoopStats
	LastResult          runonce.Result
	Progress            int64
	Err                 string
	Logs                []string
	QueueResult         autonomousqueue.Result
}

func NewStatusModel(status app.StatusResult) StatusModel {
	return NewStatusModelWithActions(status, StatusActions{})
}

func NewStatusModelWithActions(status app.StatusResult, actions StatusActions) StatusModel {
	committed := append([]transcriptCell{newSessionTranscriptCell(status.ProjectRoot, status.Initialized)}, historicalTranscriptCells(status)...)
	model := StatusModel{
		status:       status,
		actions:      actions,
		committed:    committed,
		emitted:      make(map[string]struct{}),
		view:         viewDashboard,
		previous:     viewDashboard,
		composer:     commandComposerState{Active: true},
		selectedTask: clampTaskIndex(status.Tasks, 0),
		selectedRun:  clampRunIndex(status.RecentRuns, 0),
		loopPasses:   defaultRunLoopPasses,
		focusSource:  viewDashboard,
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
		RetryTask:       opts.RetryTask,
		ValidateReceipt: opts.ValidateReceipt,
		Preflight:       opts.Preflight,
		RunOnce:         opts.RunOnce,
		RunLoop:         opts.RunLoop,
		RunTask:         opts.RunTask,
		ListAutonomous:  opts.ListAutonomous,
		LoadAutonomous:  opts.LoadAutonomous,
		AnswerInput:     opts.AnswerInput,
		RunQueue:        opts.RunQueue,
	}), options...).Run()
	return err
}

func (m StatusModel) Init() tea.Cmd {
	return m.appendCommitted()
}

func (m StatusModel) appendCommitted() tea.Cmd {
	if m.runOnce.Active {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(m.committed))
	for _, cell := range m.committed {
		if _, ok := m.emitted[cell.identity]; ok {
			continue
		}
		if m.settling != nil && cell.identity == m.settling.cell.identity {
			continue
		}
		m.emitted[cell.identity] = struct{}{}
		cmds = append(cmds, tea.Println(strings.Join(cell.render(m.width), "\n")))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Sequence(cmds...)
}

func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		m.refreshViewportContent()
	case refreshStatusMsg:
		var appendCmd tea.Cmd
		if msg.err != nil {
			m.setRefreshNotice(fmt.Sprintf("Refresh failed: %s", msg.err))
		} else {
			selectedTaskID := m.selectedTaskID()
			selectedID := m.selectedRunID()
			m.status = msg.status
			m.setSelectedTask(selectedTaskIndex(m.status.Tasks, selectedTaskID))
			m.setSelectedRun(selectedRunIndex(m.status.RecentRuns, selectedID))
			if !m.status.Initialized {
				m.runDetails = nil
				m.validation = receiptValidationState{}
			}
			m.committed = append(m.committed[:1], historicalTranscriptCells(m.status)...)
			appendCmd = m.appendCommitted()
			m.setRefreshNotice("Refreshed.")
		}
		m.updateViewportContent()
		if msg.err == nil && isFocusedView(m.view) && m.focusSource == viewRunDetail && !m.focusedFromAutonomous {
			reloadCmd := m.reloadFocusedRunCmd()
			if appendCmd != nil && reloadCmd != nil {
				return m, tea.Sequence(appendCmd, reloadCmd)
			}
			if reloadCmd != nil {
				return m, reloadCmd
			}
			return m, appendCmd
		}
		if (m.workflowActive() || isFocusedView(m.view) && m.focusedFromAutonomous) && msg.err == nil {
			loadCmd := m.loadAutonomousSelectorsCmd()
			if appendCmd != nil && loadCmd != nil {
				return m, tea.Sequence(appendCmd, loadCmd)
			}
			if loadCmd != nil {
				return m, loadCmd
			}
		}
		return m, appendCmd
	case openRunMsg:
		if msg.owner != 0 && (m.overlay == nil || m.overlay.owner != msg.owner || !m.runsOverlayActive() || m.selectedRunID() != msg.runID) {
			return m, nil
		}
		overlayDetail := false
		if msg.err != nil {
			m.setRunNotice(fmt.Sprintf("Open failed: %s", msg.err))
		} else {
			m.runDetails = &msg.history
			m.validation = receiptValidationState{RunID: strings.TrimSpace(msg.history.Run.ID)}
			if !msg.focused {
				if m.runsOverlayActive() {
					if m.overlay.content == viewRuns {
						m.overlay.parentOffset = m.overlay.viewport.YOffset
					}
					m.overlay.content = viewRunDetail
					overlayDetail = true
				} else {
					m.view = viewRunDetail
				}
			}
			m.setRunNotice("")
		}
		m.resizeViewport()
		m.updateViewportContent()
		if overlayDetail {
			m.overlay.viewport.GotoTop()
		}
		return m, nil
	case addTaskMsg:
		if msg.err != nil {
			m.taskEntry.message = fmt.Sprintf("Add failed: %s", msg.err)
		} else {
			selectedRunID := m.selectedRunID()
			m.status = msg.status
			m.setSelectedTask(selectedTaskIndex(m.status.Tasks, msg.task.ID))
			m.setSelectedRun(selectedRunIndex(m.status.RecentRuns, selectedRunID))
			if !m.status.Initialized {
				m.runDetails = nil
				m.validation = receiptValidationState{}
			}
			if m.tasksOverlayActive() {
				m.overlay.content = viewTasks
			} else {
				m.view = viewTasks
			}
			m.taskEntry = taskEntryState{}
			m.setTaskNotice(fmt.Sprintf("Added and committed task %s.", optionalValue(msg.task.ID)))
		}
		m.resizeViewport()
		m.updateViewportContent()
		return m, nil
	case retryTaskMsg:
		if msg.err != nil {
			if msg.refreshFailed {
				m.setTaskNotice(fmt.Sprintf("Retry refresh failed: %s", msg.err))
			} else {
				m.setTaskNotice(fmt.Sprintf("Retry failed: %s", msg.err))
			}
		} else {
			selectedRunID := m.selectedRunID()
			m.status = msg.status
			m.setSelectedTask(selectedTaskIndex(m.status.Tasks, msg.task.ID))
			m.setSelectedRun(selectedRunIndex(m.status.RecentRuns, selectedRunID))
			if !m.status.Initialized {
				m.runDetails = nil
				m.validation = receiptValidationState{}
			}
			m.setTaskNotice(fmt.Sprintf("Retried task %s.", optionalValue(msg.task.ID)))
		}
		m.resizeViewport()
		m.updateViewportContent()
		return m, nil
	case validateReceiptMsg:
		if msg.owner != 0 && (m.overlay == nil || m.overlay.owner != msg.owner || m.overlay.content != viewRunDetail || m.runDetails == nil || strings.TrimSpace(m.runDetails.Run.ID) != msg.runID) {
			return m, nil
		}
		m.validation = receiptValidationState{
			RunID:   msg.runID,
			Checked: true,
			Result:  msg.result,
		}
		if msg.err != nil {
			m.validation.Err = msg.err.Error()
			m.setRunNotice("Receipt validation error.")
		} else if msg.result.Passed() {
			m.setRunNotice("Receipt validation passed.")
		} else {
			m.setRunNotice("Receipt validation failed.")
		}
		m.updateViewportContent()
		return m, nil
	case preflightMsg:
		if msg.owner != 0 && (m.overlay == nil || m.overlay.owner != msg.owner || m.overlay.content != viewPreflight) {
			return m, nil
		}
		m.preflight = preflightState{
			Checked: true,
			Result:  msg.result,
		}
		if msg.err != nil {
			m.preflight.Err = msg.err.Error()
			m.setPreflightNotice("Preflight error.")
		} else if msg.result.Ready {
			m.setPreflightNotice("Preflight ready.")
		} else {
			m.setPreflightNotice("Preflight failed.")
		}
		m.updateViewportContent()
		return m, nil
	case autonomousSelectorsMsg:
		if msg.token != m.autonomous.Request {
			return m, nil
		}
		m.autonomous.LoadingList = false
		if msg.err != nil {
			m.autonomous.Err = msg.err.Error()
			m.setWorkflowNotice("Workflow selector load failed.")
			m.updateViewportContent()
			return m, nil
		}
		m.autonomous.Selectors = msg.selectors
		m.preserveAutonomousSelection()
		if len(m.autonomous.Selectors) == 0 {
			m.autonomous.View = nil
			m.autonomous.Selector = ""
			m.autonomous.TaskID = ""
			m.autonomous.Err = ""
			m.updateViewportContent()
			return m, nil
		}
		return m, m.loadSelectedAutonomousViewCmd()
	case autonomousViewMsg:
		if msg.token != m.autonomous.Request || msg.selector != m.autonomous.Selector {
			return m, nil
		}
		m.autonomous.LoadingView = false
		if msg.err != nil {
			m.autonomous.Err = msg.err.Error()
			m.setWorkflowNotice("Workflow evidence load failed.")
		} else {
			view := msg.view
			m.autonomous.View = &view
			m.autonomous.TaskID = view.Identity.TaskID
			m.autonomous.Err = ""
			m.setWorkflowNotice("Workflow evidence loaded.")
		}
		m.updateViewportContent()
		return m, nil
	case autonomousAnswerMsg:
		m.autonomous.Answer.Submitting = false
		m.autonomous.Answer.Active = false
		m.autonomous.Answer.Confirming = false
		m.autonomous.Answer.Result = msg.result
		m.autonomous.Answer.Err = ""
		if msg.err != nil {
			m.autonomous.Answer.Err = msg.err.Error()
			if msg.result.AnswerPersisted {
				m.setWorkflowNotice("Answer persisted; resume failed.")
			} else {
				m.setWorkflowNotice("Answer failed.")
			}
		} else {
			m.setWorkflowNotice("Answer persisted and task resumed.")
		}
		m.updateViewportContent()
		return m, m.reloadCurrentAutonomousViewCmd()
	case runOnceProgressMsg:
		if msg.token != m.runOnce.Token || !m.runOnce.Started {
			return m, nil
		}
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, runProgressLine(msg.event))
		m.runOnce.Current = oneLine(msg.event.Message)
		m.message = ""
		m.updateViewportContent()
		return m, m.waitRunOnceMsgCmd()
	case runLoopPassMsg:
		if msg.token != m.runOnce.Token || !m.runOnce.Started || m.runOnce.Mode != runModeLoop {
			return m, nil
		}
		m.applyRunLoopPass(msg.result)
		m.message = ""
		m.updateViewportContent()
		return m, m.waitRunOnceMsgCmd()
	case taskRunProgressMsg:
		if msg.token != m.runOnce.Token || !m.runOnce.Started || m.runOnce.Mode != runModeTask {
			return m, nil
		}
		if !m.acceptLiveOperationID(msg.operation.OperationID) {
			return m, m.waitRunOnceMsgCmd()
		}
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, fmt.Sprintf("task: cycle %d stage %s action %s", msg.operation.Statistics.CyclesStarted, msg.operation.Stage, msg.operation.LastAction))
		m.runOnce.Progress = msg.operation.Statistics.CyclesStarted
		if !msg.operation.StartedAt.IsZero() {
			m.runOnce.StartedAt = msg.operation.StartedAt
		}
		m.runOnce.Current = taskProgressDetail(msg.operation)
		m.setWorkflowNotice("")
		m.updateViewportContent()
		return m, m.waitRunOnceMsgCmd()
	case queueProgressMsg:
		if msg.token != m.runOnce.Token || !m.runOnce.Started || m.runOnce.Mode != runModeQueue {
			return m, nil
		}
		if !m.acceptLiveOperationID(msg.operation.OperationID) {
			return m, m.waitRunOnceMsgCmd()
		}
		line := fmt.Sprintf("queue: stage %s selections %d tasks %d", msg.operation.Stage, msg.operation.Statistics.Selections, msg.operation.Statistics.TasksRun)
		if msg.operation.InFlight != nil {
			line += " task " + msg.operation.InFlight.TaskID
		} else if len(msg.operation.Slots) != 0 {
			for _, slot := range msg.operation.Slots {
				line += fmt.Sprintf(" slot %d selection %d task %s operation %s", slot.Selection.Slot, slot.Selection.Sequence, slot.Selection.TaskID, slot.Selection.TaskOperationID)
			}
		}
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, line)
		m.runOnce.Progress = msg.operation.Statistics.TasksRun
		if !msg.operation.StartedAt.IsZero() {
			m.runOnce.StartedAt = msg.operation.StartedAt
		}
		m.runOnce.Current = queueProgressDetail(msg.operation)
		m.setWorkflowNotice("")
		m.updateViewportContent()
		return m, m.waitRunOnceMsgCmd()
	case runOnceDoneMsg:
		if msg.token != m.runOnce.Token || !m.runOnce.Started || m.settling != nil || !m.liveResultMatches(msg) {
			return m, nil
		}
		quitAfterSettlement := m.runOnce.QuitAfterSettlement
		cell := liveTranscriptResultCell(m.runOnce, msg)
		m.applyRunOnceDone(msg)
		m.runOnce.QuitAfterSettlement = quitAfterSettlement
		if cell.identity != "" {
			if _, ok := m.emitted[cell.identity]; !ok {
				m.settling = &transcriptSettlement{token: msg.token, cell: cell}
				m.updateViewportContent()
				return m, tea.Sequence(
					tea.Println(strings.Join(cell.render(m.width), "\n")),
					func() tea.Msg { return transcriptCommittedMsg{token: msg.token, identity: cell.identity} },
				)
			}
		}
		m.runOnce.Started = false
		m.runOnce.QuitAfterSettlement = false
		m.setWorkflowNotice("")
		m.updateViewportContent()
		if quitAfterSettlement {
			return m, tea.Quit
		}
		if m.workflowActive() && (msg.taskRun || msg.queue) {
			return m, m.loadAutonomousSelectorsCmd()
		}
		return m, nil
	case transcriptCommittedMsg:
		if m.settling == nil || msg.token != m.runOnce.Token || msg.token != m.settling.token || msg.identity != m.settling.cell.identity {
			return m, nil
		}
		cell := m.settling.cell
		m.emitted[cell.identity] = struct{}{}
		if !hasTranscriptCell(m.committed, cell.identity) {
			m.committed = append(m.committed, cell)
		}
		m.settling = nil
		m.runOnce.Started = false
		quitAfterSettlement := m.runOnce.QuitAfterSettlement
		m.runOnce.QuitAfterSettlement = false
		m.setWorkflowNotice("")
		m.updateViewportContent()
		if quitAfterSettlement {
			return m, tea.Quit
		}
		if m.workflowActive() && (m.runOnce.Mode == runModeTask || m.runOnce.Mode == runModeQueue) {
			return m, m.loadAutonomousSelectorsCmd()
		}
		return m, nil
	case tea.KeyMsg:
		if m.overlay != nil {
			return m.updateOverlay(msg)
		}
		if m.view == viewTaskEntry {
			return m.updateTaskEntry(msg)
		}
		if m.autonomous.Answer.Active {
			return m.updateAutonomousAnswer(msg)
		}
		if m.runOnce.Active {
			switch msg.String() {
			case "ctrl+c", "q", "c", "esc":
				if handled, cmd := m.updateActiveRunKeys(msg); handled {
					return m, cmd
				}
			}
		}
		if m.composer.Active {
			return m.updateCommandComposer(msg)
		}
		if msg.String() == "/" {
			m.composer = commandComposerState{Active: true, Text: "/", DiscoveryOpen: true}
			m.resizeViewport()
			return m, nil
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
			m.openTasksOverlay()
			return m, nil
		case "3":
			m.openRunsOverlay()
			return m, nil
		case "4":
			m.openRunDetailOverlay()
			return m, nil
		case "5":
			return m, m.openPreflightOverlay()
		case "6":
			return m, m.openWorkflowOverlay()
		case "?":
			m.openHelpOverlay()
			return m, nil
		case "a":
			if m.view == viewAutonomous || m.view == viewApproval {
				m.beginAutonomousAnswer()
				return m, nil
			}
			m.startTaskEntry("")
			return m, nil
		case "d":
			m.openFocusedView(viewDiff)
			return m, nil
		case "e":
			m.openFocusedView(viewEvidence)
			return m, nil
		case "A":
			m.openFocusedView(viewApproval)
			if m.autonomous.View == nil {
				return m, m.loadAutonomousSelectorsCmd()
			}
			return m, nil
		case "R":
			cmd := m.startRunOnce()
			return m, cmd
		case "n":
			m.cycleRunLoopPasses()
			return m, nil
		case "L":
			cmd := m.startRunLoop()
			return m, cmd
		case "U":
			return m, m.startTaskRun()
		case "Q":
			return m, m.startQueueRun()
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
			case viewDiff, viewEvidence, viewApproval:
				m.closeFocusedView()
				return m, nil
			}
		}
		if (m.view == viewAutonomous || isFocusedView(m.view)) && m.scrollViewport(msg) {
			return m, nil
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
			case "u":
				return m, m.startRetrySelectedTask()
			case "enter", "o":
				m.switchView(viewAutonomous)
				m.autonomous.TaskID = m.selectedTaskID()
				m.autonomous.Selector = m.selectedTaskID()
				return m, m.loadAutonomousSelectorsCmd()
			}
		case viewRuns:
			switch msg.String() {
			case "enter", "o":
				return m, m.startOpenSelectedRun()
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
				return m, m.startOpenSelectedRun()
			case "v":
				return m, m.startValidateRunReceipt()
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
		case viewAutonomous:
			switch msg.String() {
			case "up", "k":
				return m, m.moveAutonomousSelection(-1)
			case "down", "j":
				return m, m.moveAutonomousSelection(1)
			case "enter", "o":
				return m, m.reloadCurrentAutonomousViewCmd()
			case "a":
				m.beginAutonomousAnswer()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *StatusModel) scrollViewport(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "home":
		m.viewport.GotoTop()
	case "end":
		m.viewport.GotoBottom()
	case "pgup":
		m.viewport.ViewUp()
	case "pgdown":
		m.viewport.ViewDown()
	default:
		return false
	}
	return true
}

func (m StatusModel) View() string {
	content := m.viewport.View()
	if m.overlay != nil {
		content = m.overlay.viewport.View()
	}
	sections := []string{trimTrailingBlankLines(content)}
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
	runID   string
	owner   int
	focused bool
	err     error
}

type addTaskMsg struct {
	task   taskmodel.Task
	status app.StatusResult
	err    error
}

type retryTaskMsg struct {
	task          taskmodel.Task
	status        app.StatusResult
	err           error
	refreshFailed bool
}

type validateReceiptMsg struct {
	runID  string
	owner  int
	result receipt.ValidationResult
	err    error
}

type preflightMsg struct {
	result app.PreflightResult
	owner  int
	err    error
}

type runOnceProgressMsg struct {
	token int
	event codexexec.ProgressEvent
}

type runLoopPassMsg struct {
	token  int
	result runonce.Result
}

type runOnceDoneMsg struct {
	token         int
	result        runonce.Result
	loopResult    app.RunLoopResult
	err           error
	cancelled     bool
	loop          bool
	lastRunID     string
	status        app.StatusResult
	statusErr     error
	history       ledger.RunWithEvents
	historyLoaded bool
	historyErr    error
	taskRun       bool
	taskResult    autonomoustaskrun.Result
	queue         bool
	queueResult   autonomousqueue.Result
}

type transcriptCommittedMsg struct {
	token    int
	identity string
}

type taskRunProgressMsg struct {
	token     int
	operation autonomoustaskrun.Operation
}

type queueProgressMsg struct {
	token     int
	operation autonomousqueue.Operation
}

type autonomousSelectorsMsg struct {
	token     int
	selectors []app.AutonomousTaskSelector
	err       error
}

type autonomousViewMsg struct {
	token    int
	selector string
	view     autonomousview.View
	err      error
}

type autonomousAnswerMsg struct {
	result app.AnswerAutonomousInputResult
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
	index := clampRunIndex(m.status.RecentRuns, m.selectedRunPosition())
	runID := strings.TrimSpace(m.status.RecentRuns[index].ID)
	if runID == "" {
		return nil
	}
	owner := 0
	if m.runsOverlayActive() {
		owner = m.overlay.owner
	}
	return func() tea.Msg {
		history, err := m.actions.OpenRun(runID)
		return openRunMsg{history: history, runID: runID, owner: owner, err: err}
	}
}

func (m *StatusModel) startOpenSelectedRun() tea.Cmd {
	if m.runOnce.Active {
		m.setRunNotice("Run is active; cancel or wait before starting another action.")
		m.updateViewportContent()
		return nil
	}
	if m.actions.OpenRun == nil {
		m.setRunNotice("Open is unavailable.")
		m.updateViewportContent()
		return nil
	}
	cmd := m.openSelectedRunCmd()
	if cmd == nil {
		m.setRunNotice("No run selected.")
		m.updateViewportContent()
	}
	return cmd
}

func (m StatusModel) reloadFocusedRunCmd() tea.Cmd {
	if m.actions.OpenRun == nil || m.runDetails == nil {
		return nil
	}
	runID := strings.TrimSpace(m.runDetails.Run.ID)
	if runID == "" {
		return nil
	}
	return func() tea.Msg {
		history, err := m.actions.OpenRun(runID)
		return openRunMsg{history: history, runID: runID, focused: true, err: err}
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

func (m *StatusModel) startRetrySelectedTask() tea.Cmd {
	task, ok := m.selectedTaskValue()
	if !ok {
		m.setTaskNotice("No task selected.")
		m.updateViewportContent()
		return nil
	}
	if task.Status != taskmodel.StatusBlocked {
		m.setTaskNotice(fmt.Sprintf("Retry unavailable: selected task %s is not blocked (status: %s).", optionalValue(task.ID), optionalValue(task.Status)))
		m.updateViewportContent()
		return nil
	}
	if m.actions.RetryTask == nil {
		m.setTaskNotice("Retry is unavailable.")
		m.updateViewportContent()
		return nil
	}
	if m.actions.RefreshStatus == nil {
		m.setTaskNotice("Retry is unavailable: refresh callback is missing.")
		m.updateViewportContent()
		return nil
	}
	m.setTaskNotice("")
	m.updateViewportContent()
	return m.retryTaskCmd(task.ID)
}

func (m StatusModel) retryTaskCmd(taskID string) tea.Cmd {
	return func() tea.Msg {
		task, err := m.actions.RetryTask(taskID)
		if err != nil {
			return retryTaskMsg{err: err}
		}
		status, err := m.actions.RefreshStatus()
		if err != nil {
			return retryTaskMsg{task: task, err: err, refreshFailed: true}
		}
		return retryTaskMsg{task: task, status: status}
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
	owner := 0
	if m.overlay != nil && m.overlay.content == viewRunDetail {
		owner = m.overlay.owner
	}
	return func() tea.Msg {
		result, err := m.actions.ValidateReceipt(runID)
		return validateReceiptMsg{runID: runID, owner: owner, result: result, err: err}
	}
}

func (m *StatusModel) startValidateRunReceipt() tea.Cmd {
	if m.runOnce.Active {
		m.setRunNotice("Run is active; cancel or wait before starting another action.")
		m.updateViewportContent()
		return nil
	}
	if m.actions.ValidateReceipt == nil {
		m.setRunNotice("Receipt validation is unavailable.")
		m.updateViewportContent()
		return nil
	}
	cmd := m.validateRunReceiptCmd()
	if cmd == nil {
		m.setRunNotice("No run detail loaded.")
		m.updateViewportContent()
	}
	return cmd
}

func (m StatusModel) preflightCmd() tea.Cmd {
	owner := 0
	if m.preflightOverlayActive() {
		owner = m.overlay.owner
	}
	return func() tea.Msg {
		result, err := m.actions.Preflight()
		return preflightMsg{result: result, owner: owner, err: err}
	}
}

func (m *StatusModel) loadAutonomousSelectorsCmd() tea.Cmd {
	if blocker := m.autonomousListBlocker(); blocker != "" {
		m.autonomous.Err = blocker
		m.autonomous.LoadingList = false
		m.updateViewportContent()
		return nil
	}
	m.autonomous.Request++
	token := m.autonomous.Request
	m.autonomous.LoadingList = true
	m.autonomous.Err = ""
	m.updateViewportContent()
	return func() tea.Msg {
		selectors, err := m.actions.ListAutonomous()
		return autonomousSelectorsMsg{token: token, selectors: selectors, err: err}
	}
}

func (m StatusModel) autonomousListBlocker() string {
	if m.actions.ListAutonomous == nil {
		return "Workflow selector loading is unavailable."
	}
	return ""
}

func (m *StatusModel) preserveAutonomousSelection() {
	if len(m.autonomous.Selectors) == 0 {
		m.setSelectedAutonomous(0)
		return
	}
	selected := -1
	for i, item := range m.autonomous.Selectors {
		if m.autonomous.Selector != "" && item.Selector == m.autonomous.Selector {
			selected = i
			break
		}
	}
	if selected < 0 && m.autonomous.TaskID != "" {
		for i, item := range m.autonomous.Selectors {
			if item.TaskID == m.autonomous.TaskID {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		selected = clampAutonomousIndex(m.autonomous.Selectors, m.selectedAutonomousPosition())
	}
	m.setSelectedAutonomous(selected)
	item := m.autonomous.Selectors[selected]
	m.autonomous.Selector = item.Selector
	m.autonomous.TaskID = item.TaskID
}

func (m *StatusModel) loadSelectedAutonomousViewCmd() tea.Cmd {
	if len(m.autonomous.Selectors) == 0 {
		return nil
	}
	m.setSelectedAutonomous(clampAutonomousIndex(m.autonomous.Selectors, m.selectedAutonomousPosition()))
	item := m.autonomous.Selectors[m.selectedAutonomousPosition()]
	m.autonomous.Selector = item.Selector
	m.autonomous.TaskID = item.TaskID
	return m.reloadCurrentAutonomousViewCmd()
}

func (m *StatusModel) reloadCurrentAutonomousViewCmd() tea.Cmd {
	selector := strings.TrimSpace(m.autonomous.Selector)
	if selector == "" {
		return nil
	}
	if m.actions.LoadAutonomous == nil {
		m.autonomous.Err = "Workflow evidence loading is unavailable."
		m.autonomous.LoadingView = false
		m.updateViewportContent()
		return nil
	}
	m.autonomous.Request++
	token := m.autonomous.Request
	m.autonomous.LoadingView = true
	m.autonomous.Err = ""
	m.updateViewportContent()
	return func() tea.Msg {
		view, err := m.actions.LoadAutonomous(selector)
		return autonomousViewMsg{token: token, selector: selector, view: view, err: err}
	}
}

func (m *StatusModel) moveAutonomousSelection(delta int) tea.Cmd {
	if len(m.autonomous.Selectors) == 0 {
		return nil
	}
	m.setSelectedAutonomous(clampAutonomousIndex(m.autonomous.Selectors, m.selectedAutonomousPosition()+delta))
	m.autonomous.Answer = autonomousAnswerState{}
	m.autonomous.View = nil
	if m.workflowOverlayActive() {
		m.overlay.viewport.GotoTop()
	} else {
		m.viewport.GotoTop()
	}
	return m.loadSelectedAutonomousViewCmd()
}

func (m *StatusModel) beginAutonomousAnswer() {
	if blocker := m.answerBlocker(); blocker != "" {
		m.setWorkflowNotice(blocker)
		m.updateViewportContent()
		return
	}
	m.autonomous.Answer = autonomousAnswerState{Active: true, Selected: -1}
	m.setWorkflowNotice("Choose an option explicitly; the recommendation is not preselected.")
	m.updateViewportContent()
}

func (m StatusModel) answerBlocker() string {
	switch {
	case m.runOnce.Active:
		return "Run is active; cancel or wait before answering input."
	case m.actions.AnswerInput == nil:
		return "Answer input is unavailable."
	case m.autonomous.View == nil || m.autonomous.View.Identity.SourceKind != autonomousview.SourceActive || m.autonomous.View.Input.State != "waiting" || m.autonomous.View.Input.QuestionID == "" || len(m.autonomous.View.Input.Options) == 0:
		return "Answer unavailable: the selected evidence has no current typed question."
	default:
		return ""
	}
}

func (m StatusModel) updateAutonomousAnswer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.autonomous.Answer.Submitting {
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.setWorkflowNotice("Answer submission is in progress.")
		m.updateViewportContent()
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		if m.runOnce.Active {
			m.composer = commandComposerState{}
			m.runOnce.QuitAfterSettlement = true
			m.requestRunCancel()
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		m.autonomous.Answer = autonomousAnswerState{}
		m.setWorkflowNotice("Answer cancelled.")
		m.updateViewportContent()
		return m, nil
	case "up", "k":
		m.moveAutonomousAnswerOption(-1)
		return m, nil
	case "down", "j":
		m.moveAutonomousAnswerOption(1)
		return m, nil
	case "enter":
		if m.autonomous.Answer.Selected < 0 {
			m.setWorkflowNotice("Select an offered option before confirming.")
			m.updateViewportContent()
			return m, nil
		}
		if !m.autonomous.Answer.Confirming {
			if !m.confirmAutonomousAnswer(m.autonomous.Answer.Selected) {
				return m, nil
			}
			m.setWorkflowNotice("Press enter again to persist this answer and resume the task.")
			m.updateViewportContent()
			return m, nil
		}
		if !m.autonomousAnswerSnapshotCurrent() {
			m.autonomous.Answer = autonomousAnswerState{}
			m.setWorkflowNotice("Answer not submitted: the selected question changed; review the current options.")
			m.updateViewportContent()
			return m, nil
		}
		snapshot := m.autonomous.Answer.Snapshot
		m.autonomous.Answer.Submitting = true
		m.setWorkflowNotice("Persisting answer.")
		m.updateViewportContent()
		request := app.AnswerAutonomousInputRequest{TaskID: snapshot.TaskID, QuestionID: snapshot.QuestionID, Revision: snapshot.Revision, ContentSHA: snapshot.ContentSHA, OptionID: snapshot.OptionID, Operator: "tui-operator"}
		return m, func() tea.Msg {
			result, err := m.actions.AnswerInput(request)
			return autonomousAnswerMsg{result: result, err: err}
		}
	}
	return m, nil
}

func (m *StatusModel) moveAutonomousAnswerOption(delta int) {
	if m.autonomous.View == nil || len(m.autonomous.View.Input.Options) == 0 {
		return
	}
	if m.autonomous.Answer.Selected < 0 {
		if delta < 0 {
			m.autonomous.Answer.Selected = len(m.autonomous.View.Input.Options) - 1
		} else {
			m.autonomous.Answer.Selected = 0
		}
	} else {
		m.autonomous.Answer.Selected += delta
		if m.autonomous.Answer.Selected < 0 {
			m.autonomous.Answer.Selected = 0
		}
		if m.autonomous.Answer.Selected >= len(m.autonomous.View.Input.Options) {
			m.autonomous.Answer.Selected = len(m.autonomous.View.Input.Options) - 1
		}
	}
	m.autonomous.Answer.Confirming = false
	m.autonomous.Answer.Snapshot = autonomousAnswerSnapshot{}
	m.setWorkflowNotice("Option selected; press enter to review confirmation.")
	m.updateViewportContent()
}

func (m *StatusModel) confirmAutonomousAnswer(selected int) bool {
	if m.autonomous.View == nil || selected < 0 || selected >= len(m.autonomous.View.Input.Options) {
		m.autonomous.Answer = autonomousAnswerState{}
		m.setWorkflowNotice("Answer not submitted: the selected question changed; review the current options.")
		m.updateViewportContent()
		return false
	}
	view := m.autonomous.View
	m.autonomous.Answer.Selected = selected
	m.autonomous.Answer.Confirming = true
	m.autonomous.Answer.Snapshot = autonomousAnswerSnapshot{
		TaskID:     m.autonomous.TaskID,
		QuestionID: view.Input.QuestionID,
		Revision:   view.Input.Revision,
		ContentSHA: view.Input.ContentSHA256,
		OptionID:   view.Input.Options[selected].ID,
	}
	return true
}

func (m StatusModel) autonomousAnswerSnapshotCurrent() bool {
	if m.autonomous.View == nil {
		return false
	}
	snapshot := m.autonomous.Answer.Snapshot
	view := m.autonomous.View
	if view.Identity.SourceKind != autonomousview.SourceActive || view.Input.State != "waiting" ||
		m.autonomous.TaskID != snapshot.TaskID || view.Identity.TaskID != snapshot.TaskID ||
		view.Input.QuestionID != snapshot.QuestionID || view.Input.Revision != snapshot.Revision ||
		view.Input.ContentSHA256 != snapshot.ContentSHA {
		return false
	}
	for _, option := range view.Input.Options {
		if option.ID == snapshot.OptionID {
			return true
		}
	}
	return false
}

func (m *StatusModel) startRunOnce() tea.Cmd {
	if message := m.runStartBlocker(runModeOnce); message != "" {
		m.setPreflightNotice(message)
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
		Active:    true,
		Started:   true,
		Mode:      runModeOnce,
		Token:     token,
		Cancel:    cancel,
		Messages:  messages,
		Task:      m.nextLiveTaskName(),
		StartedAt: time.Now(),
		Current:   "starting the run",
		Status:    "running",
		MaxPasses: 1,
		Logs:      []string{"system: run started"},
	}
	m.setPreflightNotice("")
	m.updateViewportContent()
	return m.startRunOnceCmd(token, runCtx, messages)
}

func (m *StatusModel) startRunLoop() tea.Cmd {
	if message := m.runStartBlocker(runModeLoop); message != "" {
		m.setPreflightNotice(message)
		m.updateViewportContent()
		return nil
	}

	baseCtx := m.actions.Context
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	maxPasses := m.selectedRunLoopPasses()
	runCtx, cancel := context.WithCancel(baseCtx)
	token := m.runOnce.Token + 1
	messages := make(chan tea.Msg, 128)
	m.runOnce = runOnceState{
		Active:    true,
		Started:   true,
		Mode:      runModeLoop,
		Token:     token,
		Cancel:    cancel,
		Messages:  messages,
		Task:      m.nextLiveTaskName(),
		StartedAt: time.Now(),
		Current:   "starting pass 1",
		Status:    "running",
		MaxPasses: maxPasses,
		Stats:     app.RunLoopStats{MaxPasses: maxPasses},
		Logs:      []string{fmt.Sprintf("system: loop started (max passes %d)", maxPasses)},
	}
	m.setPreflightNotice("")
	m.updateViewportContent()
	return m.startRunLoopCmd(token, runCtx, messages, maxPasses)
}

func (m *StatusModel) startTaskRun() tea.Cmd {
	task, message := m.autonomousTaskRunBlocker()
	if message != "" {
		m.setWorkflowNotice(message)
		m.updateViewportContent()
		return nil
	}
	base := m.actions.Context
	if base == nil {
		base = context.Background()
	}
	runCtx, cancel := context.WithCancel(base)
	token := m.runOnce.Token + 1
	messages := make(chan tea.Msg, 128)
	m.runOnce = runOnceState{Active: true, Started: true, Mode: runModeTask, Token: token, Cancel: cancel, Messages: messages, Task: liveTaskName(task), StartedAt: time.Now(), Current: "starting the autonomous task", Status: "running", MaxPasses: 50, Logs: []string{"system: autonomous task run started for " + task.ID}}
	m.setWorkflowNotice("")
	m.updateViewportContent()
	actions := m.actions
	return func() tea.Msg {
		go func() {
			result, err := actions.RunTask(runCtx, task.ID, 50, func(op autonomoustaskrun.Operation) {
				select {
				case messages <- taskRunProgressMsg{token: token, operation: op}:
				case <-runCtx.Done():
				}
			})
			done := runOnceDoneMsg{token: token, taskRun: true, taskResult: result, err: err, cancelled: runCtx.Err() != nil}
			if actions.RefreshStatus != nil {
				done.status, done.statusErr = actions.RefreshStatus()
			}
			messages <- done
			close(messages)
		}()
		msg, ok := <-messages
		if !ok {
			return runOnceDoneMsg{token: token, taskRun: true, err: fmt.Errorf("run event stream closed")}
		}
		return msg
	}
}

func (m StatusModel) autonomousTaskRunBlocker() (taskmodel.Task, string) {
	if message := m.runStartBlocker(runModeTask); message != "" {
		return taskmodel.Task{}, message
	}
	task, ok := m.selectedTaskValue()
	if m.workflowActive() && m.autonomous.View != nil {
		view := m.autonomous.View
		ok = false
		if view.Identity.SourceKind == autonomousview.SourceActive {
			for _, candidate := range m.status.Tasks {
				if candidate.ID == m.autonomous.TaskID {
					task = candidate
					ok = true
					break
				}
			}
		}
	}
	notReady := ok && task.ReadinessReason != "" && !task.AutonomousReady
	if ok && m.workflowActive() && m.autonomous.View != nil && m.autonomous.View.Why.SchedulerReadiness != "ready" {
		notReady = true
		if task.ReadinessReason == "" {
			task.ReadinessReason = m.autonomous.View.Why.SchedulerReadiness
		}
	}
	if !ok || task.Workflow != taskfile.WorkflowAutonomousV1 || task.Status != taskmodel.StatusPending || notReady {
		message := "Autonomous run requires a selected pending autonomous-v1 task."
		if notReady {
			message = fmt.Sprintf("Autonomous task %s is not ready (%s).", task.ID, optionalValue(task.ReadinessReason))
		}
		return taskmodel.Task{}, message
	}
	return task, ""
}

func (m *StatusModel) startQueueRun() tea.Cmd {
	if message := m.runStartBlocker(runModeQueue); message != "" {
		m.setWorkflowNotice(message)
		m.updateViewportContent()
		return nil
	}
	base := m.actions.Context
	if base == nil {
		base = context.Background()
	}
	runCtx, cancel := context.WithCancel(base)
	token := m.runOnce.Token + 1
	messages := make(chan tea.Msg, 128)
	m.runOnce = runOnceState{Active: true, Started: true, Mode: runModeQueue, Token: token, Cancel: cancel, Messages: messages, Task: "autonomous task queue", StartedAt: time.Now(), Current: "starting the queue", Status: "running", MaxPasses: 100, Logs: []string{"system: autonomous queue started (max tasks 100, max cycles 50)"}}
	m.setWorkflowNotice("")
	m.updateViewportContent()
	actions := m.actions
	return func() tea.Msg {
		go func() {
			result, err := actions.RunQueue(runCtx, 100, 50, func(op autonomousqueue.Operation) {
				select {
				case messages <- queueProgressMsg{token: token, operation: op}:
				case <-runCtx.Done():
				}
			})
			done := runOnceDoneMsg{token: token, queue: true, queueResult: result, err: err, cancelled: runCtx.Err() != nil}
			if actions.RefreshStatus != nil {
				done.status, done.statusErr = actions.RefreshStatus()
			} else {
				done.statusErr = fmt.Errorf("refresh is unavailable")
			}
			messages <- done
			close(messages)
		}()
		msg, ok := <-messages
		if !ok {
			return runOnceDoneMsg{token: token, queue: true, err: fmt.Errorf("queue event stream closed")}
		}
		return msg
	}
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

func (m StatusModel) startRunLoopCmd(token int, ctx context.Context, messages chan tea.Msg, maxPasses int) tea.Cmd {
	actions := m.actions
	return func() tea.Msg {
		go func() {
			lastRunID := ""
			result, err := actions.RunLoop(ctx, maxPasses, func(event codexexec.ProgressEvent) {
				line := runProgressLine(event)
				if line == "" {
					return
				}
				select {
				case messages <- runOnceProgressMsg{token: token, event: event}:
				case <-ctx.Done():
				}
			}, func(result runonce.Result) error {
				if runID := strings.TrimSpace(result.Run.ID); runID != "" {
					lastRunID = runID
				}
				select {
				case messages <- runLoopPassMsg{token: token, result: result}:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})

			done := runOnceDoneMsg{
				token:      token,
				loopResult: result,
				err:        err,
				cancelled:  ctx.Err() != nil,
				loop:       true,
				lastRunID:  lastRunID,
			}
			if actions.RefreshStatus != nil {
				done.status, done.statusErr = actions.RefreshStatus()
			} else {
				done.statusErr = fmt.Errorf("refresh is unavailable")
			}
			if lastRunID != "" {
				if actions.OpenRun != nil {
					done.history, done.historyErr = actions.OpenRun(lastRunID)
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
				loop:      true,
			}
		}
		return msg
	}
}

func (m StatusModel) runStartBlocker(mode string) string {
	switch {
	case m.runOnce.Active || m.runOnce.Started:
		return "Run already active."
	case mode == runModeLoop && m.actions.RunLoop == nil:
		return "Run loop is unavailable."
	case mode == runModeTask && m.actions.RunTask == nil:
		return "Autonomous task run is unavailable."
	case mode == runModeQueue && m.actions.RunQueue == nil:
		return "Autonomous queue is unavailable."
	case mode == runModeOnce && m.actions.RunOnce == nil:
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

func (m StatusModel) commandBlocker(command string) string {
	switch command {
	case "answer":
		return m.answerBlocker()
	case "workflow":
		return m.autonomousListBlocker()
	case "approval":
		if m.autonomous.View == nil {
			return m.autonomousListBlocker()
		}
		return ""
	case "preflight":
		return m.preflightBlocker()
	case "run":
		return m.runStartBlocker(runModeOnce)
	case "loop":
		return m.runStartBlocker(runModeLoop)
	case "task-run":
		_, blocker := m.autonomousTaskRunBlocker()
		return blocker
	case "queue":
		return m.runStartBlocker(runModeQueue)
	case "refresh":
		return m.refreshBlocker()
	case "cancel":
		return m.cancelBlocker()
	case "validate":
		return m.validateBlocker()
	default:
		return ""
	}
}

func (m StatusModel) preflightBlocker() string {
	if m.runOnce.Active {
		return "Run is active; cancel or wait before checking preflight."
	}
	if m.actions.Preflight == nil {
		return "Preflight is unavailable."
	}
	return ""
}

func (m StatusModel) refreshBlocker() string {
	if m.runOnce.Active {
		return "Run is active; cancel or wait before refreshing."
	}
	if m.actions.RefreshStatus == nil {
		return "Refresh is unavailable."
	}
	return ""
}

func (m StatusModel) cancelBlocker() string {
	if !m.runOnce.Active {
		return "No active run to cancel."
	}
	return ""
}

func (m StatusModel) validateBlocker() string {
	if m.runOnce.Active {
		return "Run is active; cancel or wait before validating a receipt."
	}
	if m.actions.ValidateReceipt == nil {
		return "Receipt validation is unavailable."
	}
	if m.validateRunReceiptCmd() == nil {
		return "No run detail loaded."
	}
	return ""
}

func (m *StatusModel) acceptLiveOperationID(identity string) bool {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return true
	}
	if m.runOnce.RunID == "" {
		m.runOnce.RunID = identity
		return true
	}
	return m.runOnce.RunID == identity
}

func (m StatusModel) liveResultMatches(msg runOnceDoneMsg) bool {
	identity := ""
	switch m.runOnce.Mode {
	case runModeLoop:
		identity = msg.lastRunID
	case runModeTask:
		identity = msg.taskResult.OperationID
	case runModeQueue:
		identity = msg.queueResult.OperationID
	default:
		identity = msg.result.Run.ID
	}
	identity = strings.TrimSpace(identity)
	return identity == "" || m.runOnce.RunID == "" || identity == m.runOnce.RunID
}

func liveTranscriptResultCell(state runOnceState, msg runOnceDoneMsg) transcriptCell {
	switch state.Mode {
	case runModeTask:
		return taskRunTranscriptResultCell(state.Token, msg.taskResult, msg.err, msg.cancelled)
	case runModeQueue:
		return queueTranscriptResultCell(state.Token, msg.queueResult, msg.err, msg.cancelled)
	}

	result := msg.result
	runID := strings.TrimSpace(result.Run.ID)
	if state.Mode == runModeLoop {
		runID = strings.TrimSpace(msg.lastRunID)
		result = state.LastResult
	}
	run := result.Run
	if strings.TrimSpace(msg.history.Run.ID) == runID {
		run = msg.history.Run
	}
	for _, candidate := range msg.status.RecentRuns {
		if strings.TrimSpace(candidate.ID) == runID {
			run = candidate
			break
		}
	}
	canonical, canonicalOK := historicalRunResultCell(run)
	identity := canonical.identity
	if identity == "" {
		terminal := runTerminalStatus(msg.result, msg.err, msg.cancelled)
		if runID != "" {
			status := strings.TrimSpace(run.Status)
			if status == "" {
				status = ledger.StatusFailed
				if terminal == "completed" {
					status = ledger.StatusCompleted
				}
			}
			identity = "run:" + runID + ":status:" + status
		} else {
			identity = fmt.Sprintf("operation:%s:%d:status:%s", state.Mode, state.Token, terminal)
		}
	}
	name := oneLine(run.Task)
	if name == "" {
		name = strings.TrimSpace(result.Task.ID)
	}
	if name == "" {
		name = oneLine(state.Task)
	}
	if name == "" {
		name = "run"
	}
	reason := oneLine(result.Message)
	if reason == "" && msg.err != nil {
		reason = oneLine(msg.err.Error())
	}
	switch {
	case msg.cancelled:
		return terminalTranscriptCell(identity, name, "cancelled", reason)
	case result.Receipt.Verdict == receipt.VerdictSafetyLimit:
		return terminalTranscriptCell(identity, name, "safety_stop", reason)
	case result.Outcome == runonce.OutcomeBlocked:
		return terminalTranscriptCell(identity, name, "blocked", reason)
	case canonicalOK:
		return canonical
	case result.NoTask || result.Outcome == runonce.OutcomeNoTask:
		return transcriptCell{kind: transcriptCellStatus, identity: identity, source: []string{"No pending runnable tasks.", "Next: add a task or wait for dependencies"}}
	case msg.err != nil || app.RunOnceOutcomeError(result) != nil:
		return terminalTranscriptCell(identity, name, "failed", reason)
	default:
		return terminalTranscriptCell(identity, name, "completed", reason)
	}
}

func taskRunTranscriptResultCell(token int, result autonomoustaskrun.Result, err error, cancelled bool) transcriptCell {
	identity := strings.TrimSpace(result.OperationID)
	if identity == "" {
		identity = fmt.Sprintf("task:%s:%d", strings.TrimSpace(result.TaskID), token)
	}
	identity = "task-operation:" + identity + ":stop:" + optionalValue(string(result.StopReason))
	name := optionalValue(strings.TrimSpace(result.TaskID))
	reason := oneLine(result.StopDetail)
	if reason == "" && err != nil {
		reason = oneLine(err.Error())
	}
	outcome := "failed"
	switch result.StopReason {
	case autonomoustaskrun.StopCompleted:
		outcome = "completed"
	case autonomoustaskrun.StopBlocked:
		outcome = "blocked"
	case autonomoustaskrun.StopNeedsInput:
		outcome = "needs_input"
	case autonomoustaskrun.StopSafety:
		outcome = "safety_stop"
	case autonomoustaskrun.StopTaskCancelled, autonomoustaskrun.StopOperationCancelled:
		outcome = "cancelled"
	}
	if cancelled {
		outcome = "cancelled"
	}
	return terminalTranscriptCell(identity, name, outcome, reason)
}

func queueTranscriptResultCell(token int, result autonomousqueue.Result, err error, cancelled bool) transcriptCell {
	operationID := strings.TrimSpace(result.OperationID)
	if operationID == "" {
		operationID = fmt.Sprintf("queue:%d", token)
	}
	identity := "queue-operation:" + operationID + ":stop:" + optionalValue(string(result.StopReason))
	name := "queue " + operationID
	reason := oneLine(result.StopDetail)
	if reason == "" && err != nil {
		reason = oneLine(err.Error())
	}
	outcome := "failed"
	switch result.StopReason {
	case autonomousqueue.StopDrained:
		outcome = "completed"
	case autonomousqueue.StopWaitingDependency, autonomousqueue.StopWaitingBlocked, autonomousqueue.StopBudgetExhausted:
		outcome = "blocked"
	case autonomousqueue.StopWaitingInput:
		outcome = "needs_input"
	case autonomousqueue.StopSafety:
		outcome = "safety_stop"
	case autonomousqueue.StopCancelled:
		outcome = "cancelled"
	}
	if cancelled {
		outcome = "cancelled"
	}
	return terminalTranscriptCell(identity, name, outcome, reason)
}

func terminalTranscriptCell(identity string, name string, outcome string, reason string) transcriptCell {
	cell := transcriptCell{kind: transcriptCellWarning, identity: identity}
	switch outcome {
	case "completed":
		cell.kind = transcriptCellResult
		cell.source = []string{"Completed: " + name, "Next: /run to continue"}
	case "cancelled":
		cell.source = []string{"Cancelled: " + name, "Result: no completion was recorded", "Next: /run to retry"}
	case "blocked":
		cell.source = []string{"Blocked: " + name, "Reason: " + optionalValue(reason), "Next: /workflow to inspect the task"}
	case "safety_stop":
		cell.source = []string{"Safety stop: " + name, "Reason: " + optionalValue(reason), "Next: /detail to inspect the evidence"}
	case "needs_input":
		cell.kind = transcriptCellQuestion
		cell.source = []string{"Needs input: " + name, "Question: " + optionalValue(reason), "Next: answer the question to continue"}
	default:
		cell.source = []string{"Failed: " + name, "Reason: " + optionalValue(reason), "Next: /detail to inspect the failure"}
	}
	return cell
}

func hasTranscriptCell(cells []transcriptCell, identity string) bool {
	for _, cell := range cells {
		if cell.identity == identity {
			return true
		}
	}
	return false
}

func (m *StatusModel) updateActiveRunKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.runOnce.Active {
		return false, nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		m.runOnce.QuitAfterSettlement = true
		m.requestRunCancel()
		return true, nil
	case "c", "esc":
		m.requestRunCancel()
		return true, nil
	case "R", "L", "U", "Q", "n", "r", "a", "u":
		m.setWorkflowNotice("Run is active; cancel or wait before starting another action.")
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
		return
	}
	m.runOnce.CancelRequested = true
	if m.runOnce.Cancel != nil {
		m.runOnce.Cancel()
	}
	m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: cancellation requested")
	m.setWorkflowNotice("")
	m.updateViewportContent()
}

func (m *StatusModel) applyRunLoopPass(result runonce.Result) {
	stats := m.runOnce.Stats
	if stats.MaxPasses <= 0 {
		stats.MaxPasses = m.runOnce.MaxPasses
	}
	stats.Passes++
	if result.NoTask || result.Outcome == runonce.OutcomeNoTask {
		stats.NoTask = true
		stats.ConsecutiveFailedOrBlocked = 0
	} else if err := app.RunOnceOutcomeError(result); err != nil {
		stats.FailedOrBlocked++
		stats.ConsecutiveFailedOrBlocked++
	} else {
		if result.Outcome == runonce.OutcomeCommitted {
			stats.Completed++
		}
		stats.ConsecutiveFailedOrBlocked = 0
	}
	m.runOnce.Stats = stats
	m.runOnce.LastResult = result
	if name := liveRunResultName(result); name != "" {
		m.runOnce.Task = name
	}
	m.runOnce.Current = runResultSummary(result)
	if runID := strings.TrimSpace(result.Run.ID); runID != "" {
		m.runOnce.RunID = runID
	}
	m.runOnce.Logs = appendRunLog(m.runOnce.Logs, runPassSummaryLine(stats.Passes, result))
}

func (m *StatusModel) applyRunOnceDone(msg runOnceDoneMsg) {
	if msg.queue || m.runOnce.Mode == runModeQueue {
		m.runOnce.Active = false
		m.runOnce.Cancel = nil
		m.runOnce.Messages = nil
		m.runOnce.RunID = msg.queueResult.OperationID
		m.runOnce.Outcome = string(msg.queueResult.StopReason)
		m.runOnce.Status = string(msg.queueResult.StopReason)
		m.runOnce.QueueResult = msg.queueResult
		m.runOnce.Err = ""
		if msg.err != nil {
			m.runOnce.Err = msg.err.Error()
		}
		if msg.cancelled && m.runOnce.Status == "" {
			m.runOnce.Status = "cancelled"
			m.runOnce.Outcome = "cancelled"
		}
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: terminal state: "+optionalValue(m.runOnce.Status))
		m.setWorkflowNotice("Autonomous queue stopped: " + optionalValue(m.runOnce.Status) + ".")
		if msg.statusErr == nil {
			selectedTaskID := m.selectedTaskID()
			m.status = msg.status
			m.setSelectedTask(selectedTaskIndex(m.status.Tasks, selectedTaskID))
		} else {
			m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: refresh failed: "+oneLine(msg.statusErr.Error()))
		}
		return
	}
	if msg.taskRun || m.runOnce.Mode == runModeTask {
		m.runOnce.Active = false
		m.runOnce.Cancel = nil
		m.runOnce.Messages = nil
		m.runOnce.RunID = msg.taskResult.LastRunID
		m.runOnce.Outcome = string(msg.taskResult.StopReason)
		m.runOnce.Err = ""
		if msg.err != nil {
			m.runOnce.Err = msg.err.Error()
		}
		m.runOnce.Status = string(msg.taskResult.StopReason)
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: terminal state: "+string(msg.taskResult.StopReason))
		m.setWorkflowNotice(fmt.Sprintf("Autonomous task %s stopped: %s.", msg.taskResult.TaskID, msg.taskResult.StopReason))
		if msg.statusErr == nil {
			m.status = msg.status
			m.setSelectedTask(selectedTaskIndex(m.status.Tasks, msg.taskResult.TaskID))
		}
		return
	}
	if msg.loop || m.runOnce.Mode == runModeLoop {
		m.applyRunLoopDone(msg)
		return
	}

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

func (m *StatusModel) cycleRunLoopPasses() {
	current := m.selectedRunLoopPasses()
	next := runLoopPassOptions[0]
	for i, option := range runLoopPassOptions {
		if option == current {
			next = runLoopPassOptions[(i+1)%len(runLoopPassOptions)]
			break
		}
	}
	m.loopPasses = next
	m.setPreflightNotice(fmt.Sprintf("Loop max passes set to %d.", next))
	m.updateViewportContent()
}

func (m StatusModel) selectedRunLoopPasses() int {
	if m.loopPasses > 0 {
		return m.loopPasses
	}
	return defaultRunLoopPasses
}

func (m *StatusModel) applyRunLoopDone(msg runOnceDoneMsg) {
	runID := strings.TrimSpace(msg.lastRunID)
	stats := msg.loopResult.Stats
	if stats.MaxPasses <= 0 {
		stats.MaxPasses = m.runOnce.MaxPasses
	}
	if msg.cancelled && strings.TrimSpace(stats.StopReason) == "" {
		stats.StopReason = "context_cancelled"
	}
	terminal := runLoopTerminalStatus(stats, msg.err, msg.cancelled)

	m.runOnce.Active = false
	m.runOnce.Cancel = nil
	m.runOnce.Messages = nil
	m.runOnce.Status = terminal
	m.runOnce.RunID = runID
	m.runOnce.Outcome = ""
	m.runOnce.Stats = stats
	m.runOnce.Err = ""
	if msg.err != nil {
		m.runOnce.Err = msg.err.Error()
	}
	if stopReason := strings.TrimSpace(stats.StopReason); stopReason != "" {
		m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: stop reason: "+stopReason)
	}
	m.runOnce.Logs = appendRunLog(m.runOnce.Logs, "system: terminal state: "+terminal)

	switch terminal {
	case "cancelled":
		m.message = "Loop cancelled."
	case "completed":
		m.message = "Loop completed."
	case "no_task":
		m.message = "Loop finished: no pending runnable tasks."
	default:
		m.message = "Loop failed."
	}
	if runID != "" {
		m.message += " Latest run " + runID + "."
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
	m.setSelectedTask(selectedTaskIndex(m.status.Tasks, selectedTaskID))
	m.setSelectedRun(selectedRunIndex(m.status.RecentRuns, selectedRunID))
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

func (m StatusModel) updateCommandComposer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.runOnce.Active {
			m.runOnce.QuitAfterSettlement = true
			m.requestRunCancel()
			m.message = "Cancellation requested; waiting for run settlement before exit."
			m.updateViewportContent()
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if m.composer.DiscoveryOpen {
			m.composer.DiscoveryOpen = false
			m.resizeViewport()
			return m, nil
		}
		if m.composer.Text != "" {
			return m, nil
		}
		m.composer.Active = false
		m.resizeViewport()
		return m, nil
	case "up":
		if m.composer.DiscoveryOpen {
			m.moveCommandSelection(-1)
			return m, nil
		}
	case "down":
		if m.composer.DiscoveryOpen {
			m.moveCommandSelection(1)
			return m, nil
		}
	case "backspace":
		m.composer.Text = trimLastRune(m.composer.Text)
		m.updateCommandDiscovery()
		m.resizeViewport()
		return m, nil
	case "enter":
		if m.composer.DiscoveryOpen && !(m.composer.Text == "/" && m.composer.SelectedCommand == 0) {
			if command, ok := m.selectedSlashCommand(); ok {
				if blocker := m.commandBlocker(command.name); blocker != "" {
					m.message = blocker
					m.updateViewportContent()
					return m, nil
				}
				m.composer.Text = replaceSlashCommand(m.composer.Text, command.name)
			}
		}
		return m.submitCommand()
	case "?":
		if m.composer.Text == "" {
			m.openHelpOverlay()
			return m, nil
		}
	}
	if msg.Type == tea.KeyRunes {
		m.composer.Text += string(msg.Runes)
		m.updateCommandDiscovery()
		m.resizeViewport()
	} else if msg.Type == tea.KeySpace {
		m.composer.Text += " "
		m.updateCommandDiscovery()
		m.resizeViewport()
	}
	return m, nil
}

func (m *StatusModel) updateCommandDiscovery() {
	m.composer.DiscoveryOpen = strings.HasPrefix(m.composer.Text, "/")
	m.composer.SelectedCommand = 0
}

func (m *StatusModel) moveCommandSelection(delta int) {
	commands := m.filteredSlashCommands()
	if len(commands) == 0 {
		m.composer.SelectedCommand = 0
		return
	}
	m.composer.SelectedCommand = min(max(m.composer.SelectedCommand+delta, 0), len(commands)-1)
}

func (m StatusModel) selectedSlashCommand() (slashCommand, bool) {
	commands := m.filteredSlashCommands()
	if len(commands) == 0 {
		return slashCommand{}, false
	}
	return commands[min(max(m.composer.SelectedCommand, 0), len(commands)-1)], true
}

func (m StatusModel) filteredSlashCommands() []slashCommand {
	if !m.composer.DiscoveryOpen || !strings.HasPrefix(m.composer.Text, "/") {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(strings.Fields(m.composer.Text + " ")[0], "/"))
	commands := make([]slashCommand, 0, len(slashCommands))
	for _, command := range slashCommands {
		if command.name == prefix {
			commands = append(commands, command)
		}
	}
	for _, command := range slashCommands {
		if command.name != prefix && strings.HasPrefix(command.name, prefix) {
			commands = append(commands, command)
		}
	}
	return commands
}

func replaceSlashCommand(text, name string) string {
	if index := strings.IndexAny(text, " \t\r\n"); index >= 0 {
		return "/" + name + text[index:]
	}
	return "/" + name
}

func (m StatusModel) submitCommand() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.composer.Text)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return m, nil
	}
	if !strings.HasPrefix(fields[0], "/") {
		switch {
		case !m.status.Initialized:
			m.message = "Input unavailable: run revolvr init first"
		case m.runOnce.Active || m.runOnce.Started:
			m.message = "Input unavailable: active steering is not supported"
			if m.runOnce.Mode != "" && m.runOnce.Mode != runModeOnce {
				m.message = "Input unavailable: queued or deferred input is not supported"
			}
		case m.actions.AddTask == nil:
			m.message = "Input unavailable: add task is unavailable"
		case m.actions.RefreshStatus == nil:
			m.message = "Input unavailable: refresh is unavailable"
		default:
			draft := m.composer.Text
			m.composer.Text = ""
			m.startTaskEntry(draft)
			return m, nil
		}
		m.updateViewportContent()
		return m, nil
	}
	if fields[0] == "/" {
		m.openHelpOverlay()
		m.message = "Slash commands opened."
		m.updateViewportContent()
		return m, nil
	}
	m.composer.Text = ""
	m.composer.DiscoveryOpen = false
	m.resizeViewport()
	command := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	switch command {
	case "help", "commands":
		m.openHelpOverlay()
		return m, nil
	case "dashboard", "transcript":
		m.switchView(viewDashboard)
		return m, nil
	case "tasks":
		m.openTasksOverlay()
		return m, nil
	case "runs":
		m.openRunsOverlay()
		return m, nil
	case "detail":
		m.openRunDetailOverlay()
		return m, nil
	case "preflight":
		return m, m.openPreflightOverlay()
	case "workflow":
		return m, m.openWorkflowOverlay()
	case "diff":
		m.openFocusedView(viewDiff)
		return m, nil
	case "evidence":
		m.openFocusedView(viewEvidence)
		return m, nil
	case "approval":
		m.openFocusedView(viewApproval)
		if m.autonomous.View == nil {
			return m, m.loadAutonomousSelectorsCmd()
		}
		return m, nil
	case "answer":
		if len(fields) != 2 {
			m.message = "Usage: /answer <option-id>."
			m.updateViewportContent()
			return m, nil
		}
		m.openFocusedView(viewApproval)
		m.beginAutonomousAnswer()
		if !m.autonomous.Answer.Active {
			return m, nil
		}
		for i, option := range m.autonomous.View.Input.Options {
			if option.ID == fields[1] {
				if !m.confirmAutonomousAnswer(i) {
					return m, nil
				}
				m.message = "Press enter to persist this answer and resume the task."
				m.updateViewportContent()
				return m, nil
			}
		}
		m.autonomous.Answer = autonomousAnswerState{}
		m.message = "Answer option not offered: " + oneLine(fields[1])
		m.updateViewportContent()
		return m, nil
	case "refresh":
		if blocker := m.refreshBlocker(); blocker != "" {
			m.message = blocker
			m.updateViewportContent()
			return m, nil
		}
		return m, m.refreshStatusCmd()
	case "cancel":
		if blocker := m.cancelBlocker(); blocker != "" {
			m.message = blocker
			m.updateViewportContent()
			return m, nil
		}
		m.requestRunCancel()
		return m, nil
	case "run":
		return m, m.startRunOnce()
	case "loop":
		return m, m.startRunLoop()
	case "task-run":
		return m, m.startTaskRun()
	case "queue":
		return m, m.startQueueRun()
	case "validate":
		if blocker := m.validateBlocker(); blocker != "" {
			m.message = blocker
			m.updateViewportContent()
			return m, nil
		}
		return m, m.validateRunReceiptCmd()
	case "quit":
		if m.runOnce.Active {
			m.runOnce.QuitAfterSettlement = true
			m.requestRunCancel()
			return m, nil
		}
		return m, tea.Quit
	default:
		m.message = "Unknown command: /" + command + ". Press / then enter for help."
		m.updateViewportContent()
		return m, nil
	}
}

func (m *StatusModel) updateViewportContent() {
	m.viewport.SetContent(m.formatContent(m.renderContent()))
	m.viewport.GotoTop()
	m.refreshOverlayContent()
}

func (m *StatusModel) refreshViewportContent() {
	m.viewport.SetContent(m.formatContent(m.renderContent()))
	m.refreshOverlayContent()
}

func (m *StatusModel) openOverlay(content TUIView, selected int) {
	if m.overlay != nil {
		return
	}
	m.overlayOwner++
	m.overlay = &overlayState{
		content:      content,
		source:       m.view,
		composer:     m.composer,
		selected:     selected,
		sourceOffset: m.viewport.YOffset,
		owner:        m.overlayOwner,
		viewport:     viewport.New(defaultViewportWidth, defaultViewportHeight),
	}
	m.composer.Active = false
	m.composer.DiscoveryOpen = false
	m.resizeViewport()
	m.refreshOverlayContent()
	m.overlay.viewport.GotoTop()
}

func (m *StatusModel) openHelpOverlay() {
	m.openOverlay(viewHelp, 0)
}

func (m *StatusModel) openTasksOverlay() {
	m.openOverlay(viewTasks, m.selectedTask)
}

func (m *StatusModel) openRunsOverlay() {
	m.openOverlay(viewRuns, m.selectedRun)
}

func (m *StatusModel) openRunDetailOverlay() {
	m.openRunsOverlay()
	if m.overlay == nil || m.overlay.content != viewRuns {
		return
	}
	m.overlay.parentOffset = m.overlay.viewport.YOffset
	m.overlay.content = viewRunDetail
	m.refreshOverlayContent()
	m.overlay.viewport.GotoTop()
}

func (m *StatusModel) openPreflightOverlay() tea.Cmd {
	m.openOverlay(viewPreflight, 0)
	if !m.preflightOverlayActive() {
		return nil
	}
	if blocker := m.preflightBlocker(); blocker != "" {
		m.setPreflightNotice(blocker)
		m.refreshOverlayContent()
		return nil
	}
	return m.preflightCmd()
}

func (m *StatusModel) openWorkflowOverlay() tea.Cmd {
	m.openOverlay(viewAutonomous, m.autonomous.Selected)
	if !m.workflowOverlayActive() {
		return nil
	}
	return m.loadAutonomousSelectorsCmd()
}

func (m StatusModel) updateOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.overlay.content == viewTaskEntry {
		return m.updateTaskEntry(msg)
	}
	if m.overlay.content == viewAutonomous && m.runOnce.Active && msg.String() == "esc" {
		_, cmd := m.updateActiveRunKeys(msg)
		return m, cmd
	}
	if m.overlay.content == viewAutonomous && m.autonomous.Answer.Active {
		return m.updateAutonomousAnswer(msg)
	}
	if m.overlay.content == viewTasks {
		switch msg.String() {
		case "up", "k":
			m.moveSelectedTask(-1)
			m.refreshOverlayContent()
			return m, nil
		case "down", "j":
			m.moveSelectedTask(1)
			m.refreshOverlayContent()
			return m, nil
		case "a":
			m.startTaskEntry("")
			return m, nil
		case "u":
			return m, m.startRetrySelectedTask()
		case "enter", "o":
			taskID := m.selectedTaskID()
			m.closeOverlay()
			m.autonomous.TaskID = taskID
			m.autonomous.Selector = taskID
			return m, m.openWorkflowOverlay()
		case "r":
			if m.actions.RefreshStatus == nil {
				m.setTaskNotice("Refresh is unavailable.")
				m.refreshOverlayContent()
				return m, nil
			}
			return m, m.refreshStatusCmd()
		}
	}
	if m.overlay.content == viewRuns || m.overlay.content == viewRunDetail {
		if msg.String() == "r" {
			if m.runOnce.Active {
				m.setRunNotice("Run is active; cancel or wait before starting another action.")
				m.refreshOverlayContent()
				return m, nil
			}
			if m.actions.RefreshStatus == nil {
				m.setRunNotice("Refresh is unavailable.")
				m.refreshOverlayContent()
				return m, nil
			}
			return m, m.refreshStatusCmd()
		}
	}
	if m.overlay.content == viewRuns {
		switch msg.String() {
		case "up", "k":
			m.moveSelectedRun(-1)
			m.refreshOverlayContent()
			return m, nil
		case "down", "j":
			m.moveSelectedRun(1)
			m.refreshOverlayContent()
			return m, nil
		case "enter", "o":
			return m, m.startOpenSelectedRun()
		}
	}
	if m.overlay.content == viewRunDetail {
		switch msg.String() {
		case "esc", "backspace":
			m.overlay.content = viewRuns
			m.refreshOverlayContent()
			m.overlay.viewport.SetYOffset(m.overlay.parentOffset)
			return m, nil
		case "enter", "o":
			return m, m.startOpenSelectedRun()
		case "v":
			return m, m.startValidateRunReceipt()
		}
	}
	if m.overlay.content == viewPreflight {
		switch msg.String() {
		case "p":
			if blocker := m.preflightBlocker(); blocker != "" {
				m.setPreflightNotice(blocker)
				m.refreshOverlayContent()
				return m, nil
			}
			return m, m.preflightCmd()
		case "r":
			if blocker := m.refreshBlocker(); blocker != "" {
				m.setPreflightNotice(blocker)
				m.refreshOverlayContent()
				return m, nil
			}
			return m, m.refreshStatusCmd()
		case "R":
			return m, m.startRunOnce()
		case "n":
			if m.runOnce.Active {
				m.setPreflightNotice("Run is active; cancel or wait before starting another action.")
				m.refreshOverlayContent()
				return m, nil
			}
			m.cycleRunLoopPasses()
			return m, nil
		case "L":
			return m, m.startRunLoop()
		}
	}
	if m.overlay.content == viewAutonomous {
		switch msg.String() {
		case "up", "k":
			return m, m.moveAutonomousSelection(-1)
		case "down", "j":
			return m, m.moveAutonomousSelection(1)
		case "enter", "o":
			return m, m.reloadCurrentAutonomousViewCmd()
		case "a":
			m.beginAutonomousAnswer()
			return m, nil
		case "U":
			return m, m.startTaskRun()
		case "Q":
			return m, m.startQueueRun()
		case "r":
			if blocker := m.refreshBlocker(); blocker != "" {
				m.setWorkflowNotice(blocker)
				m.refreshOverlayContent()
				return m, nil
			}
			return m, m.refreshStatusCmd()
		}
	}
	switch msg.String() {
	case "esc", "backspace":
		m.closeOverlay()
		return m, nil
	case "ctrl+c", "q", "c":
		if handled, cmd := m.updateActiveRunKeys(msg); handled {
			return m, cmd
		}
		if msg.String() != "c" {
			return m, tea.Quit
		}
		return m, nil
	case "?":
		return m, nil
	case "home":
		m.overlay.viewport.GotoTop()
		return m, nil
	case "end":
		m.overlay.viewport.GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.overlay.viewport, cmd = m.overlay.viewport.Update(msg)
	m.overlay.selected = max(m.overlay.selected, 0)
	return m, cmd
}

func (m *StatusModel) closeOverlay() {
	if m.overlay == nil {
		return
	}
	overlay := *m.overlay
	if overlay.content == viewAutonomous {
		m.autonomous.Request++
		m.autonomous.LoadingList = false
		m.autonomous.LoadingView = false
	}
	m.overlay = nil
	m.view = overlay.source
	m.composer = overlay.composer
	m.resizeViewport()
	m.refreshViewportContent()
	m.viewport.SetYOffset(overlay.sourceOffset)
}

func (m *StatusModel) refreshOverlayContent() {
	if m.overlay == nil {
		return
	}
	m.resizeViewport()
	offset := m.overlay.viewport.YOffset
	m.overlay.viewport.SetContent(m.formatContent(m.renderOverlayContent()))
	m.overlay.viewport.SetYOffset(offset)
}

func (m StatusModel) renderOverlayContent() string {
	if m.overlay == nil {
		return ""
	}
	switch m.overlay.content {
	case viewHelp:
		return m.renderHelp()
	case viewTasks:
		return m.renderTasks()
	case viewRuns:
		return m.renderRuns()
	case viewRunDetail:
		if m.runDetails != nil {
			return m.renderRunDetails(*m.runDetails)
		}
		return m.renderEmptyRunDetail()
	case viewPreflight:
		return m.renderPreflight()
	case viewAutonomous:
		content := m.renderAutonomousWorkflow()
		if m.runOnce.Started {
			return lipgloss.JoinVertical(lipgloss.Left, m.renderRunProgress(), "", content)
		}
		return content
	case viewTaskEntry:
		return m.renderTaskEntry()
	}
	return ""
}

func (m *StatusModel) switchView(view TUIView) {
	if m.view != view && view != viewHelp {
		m.previous = m.view
	}
	if view == viewHelp && m.view != viewHelp {
		m.previous = m.view
	}
	m.view = view
	m.composer.Active = view == viewDashboard
	m.composer.DiscoveryOpen = false
	m.resizeViewport()
	m.updateViewportContent()
}

func (m *StatusModel) openFocusedView(view TUIView) {
	if !isFocusedView(m.view) {
		m.focusSource = m.view
		m.focusedFromAutonomous = m.view == viewAutonomous
	}
	if view == viewApproval {
		m.focusedFromAutonomous = true
	}
	m.view = view
	m.composer.Active = false
	m.composer.DiscoveryOpen = false
	m.resizeViewport()
	m.updateViewportContent()
}

func (m *StatusModel) closeFocusedView() {
	view := m.focusSource
	if isFocusedView(view) || view == viewTaskEntry {
		view = viewDashboard
	}
	m.focusedFromAutonomous = false
	m.switchView(view)
}

func isFocusedView(view TUIView) bool {
	return view == viewDiff || view == viewEvidence || view == viewApproval
}

func (m *StatusModel) startTaskEntry(taskText string) {
	m.taskEntry = taskEntryState{
		previous: m.view,
		field:    taskEntryTaskField,
		taskText: taskText,
	}
	if m.tasksOverlayActive() {
		m.overlay.content = viewTaskEntry
		m.overlay.message = ""
	} else {
		m.view = viewTaskEntry
	}
	m.composer.Active = false
	m.composer.DiscoveryOpen = false
	if !m.tasksOverlayActive() {
		m.message = ""
	}
	m.resizeViewport()
	m.updateViewportContent()
}

func (m *StatusModel) cancelTaskEntry() {
	if m.tasksOverlayActive() {
		m.overlay.content = viewTasks
		m.taskEntry = taskEntryState{}
		m.resizeViewport()
		m.refreshOverlayContent()
		return
	}
	previous := m.taskEntry.previous
	if previous == viewTaskEntry {
		previous = viewTasks
	}
	m.view = previous
	m.composer.Active = previous == viewDashboard
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
	chromeHeight := len(m.footerLines()) + 1
	contentHeight := max(height-chromeHeight, 1)
	m.viewport.Width = width
	m.viewport.Height = contentHeight
	if m.overlay != nil {
		m.overlay.viewport.Width = width
		m.overlay.viewport.Height = contentHeight
	}
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
	if m.settling != nil {
		return m.renderRunProgress()
	}
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
	case viewAutonomous:
		content = m.renderAutonomousWorkflow()
	case viewHelp:
		content = m.renderHelp()
	case viewTaskEntry:
		content = m.renderTaskEntry()
	case viewDiff:
		content = m.renderFocusedDiff()
	case viewEvidence:
		content = m.renderFocusedEvidence()
	case viewApproval:
		content = m.renderFocusedApproval()
	default:
		content = m.renderDashboard()
	}
	if m.runOnce.Started && m.view != viewHelp && m.view != viewTaskEntry {
		return lipgloss.JoinVertical(lipgloss.Left, m.renderRunProgress(), "", content)
	}
	return content
}

func (m StatusModel) footerLines() []string {
	if m.overlay != nil {
		if m.overlay.content == viewTaskEntry {
			return wrapKeyLines([]string{"tab Field", "enter Submit", "esc Cancel", "ctrl+c Quit"}, m.width)
		}
		if m.overlay.content == viewTasks {
			keys := []string{"j/k Select", "enter Workflow"}
			if m.retrySelectedTaskAvailable() {
				keys = append(keys, "u Retry")
			}
			keys = append(keys, "a Add Task", "r Refresh", "esc Close", "q Quit")
			return wrapKeyLines(keys, m.width)
		}
		if m.overlay.content == viewRuns {
			return wrapKeyLines([]string{"j/k Select", "enter Open", "r Refresh", "esc Close", "q Quit"}, m.width)
		}
		if m.overlay.content == viewRunDetail {
			return wrapKeyLines([]string{"up/down Scroll", "home/end Jump", "enter Reload", "v Validate", "r Refresh", "esc Runs", "q Quit"}, m.width)
		}
		if m.overlay.content == viewPreflight {
			return wrapKeyLines([]string{"p Check", "R Run Once", fmt.Sprintf("n Passes %d", m.selectedRunLoopPasses()), "L Run Loop", "r Refresh", "esc Close", "q Quit"}, m.width)
		}
		if m.overlay.content == viewAutonomous {
			keys := []string{"j/k Select", "enter Reload", "a Answer", "U Run Task", "Q Run Queue", "r Refresh", "pgup/pgdown Scroll", "home/end Jump"}
			if !m.runOnce.Active {
				keys = append(keys, "esc Close")
			}
			return wrapKeyLines(append(keys, "q Quit"), m.width)
		}
		return wrapPlainLines([]string{"↑/↓ scroll · Esc close"}, m.contentWidth())
	}
	if m.view == viewTaskEntry {
		return wrapKeyLines([]string{"tab Field", "enter Submit", "esc Cancel", "ctrl+c Quit"}, m.width)
	}
	if m.autonomous.Answer.Active {
		return wrapKeyLines([]string{"j/k Choose option", "enter Confirm", "esc Cancel answer", "ctrl+c Quit"}, m.width)
	}
	if m.composer.Active {
		lines := m.commandDiscoveryLines()
		composer := wrapPlainLines([]string{"› " + m.composer.Text}, m.contentWidth())
		footer := []string{"Enter submit · / commands · ? shortcuts"}
		if m.contentWidth() <= 40 {
			footer = []string{"Enter submit · / commands", "? shortcuts"}
		}
		lines = append(lines, composer...)
		return append(lines, wrapPlainLines(footer, m.contentWidth())...)
	}
	if m.view == viewDashboard {
		keys := "? Help | R Run | r Refresh | q Quit"
		if m.runOnce.Active {
			keys = "? Help | q Quit"
		}
		return append([]string{"›"}, wrapPlainLines([]string{keys}, m.contentWidth())...)
	}
	keys := []string{}
	if m.runOnce.Active {
		switch m.view {
		case viewTasks, viewRuns:
			keys = append(keys, "j/k Select")
		case viewRunDetail:
			keys = append(keys, "up/down Scroll", "home/end Jump")
		case viewHelp:
			keys = append(keys, "esc Back")
		}
		keys = append(keys, "1 Dashboard", "2 Tasks", "3 Runs", "4 Detail", "5 Preflight", "? Help")
		if m.view == viewHelp {
			keys = append(keys, "c Cancel Run")
		}
		keys = append(keys, "q Quit")
		return wrapKeyLines(keys, m.width)
	}
	switch m.view {
	case viewTasks:
		keys = append(keys, "j/k Select")
		if m.retrySelectedTaskAvailable() {
			keys = append(keys, "u Retry")
		}
	case viewRuns:
		keys = append(keys, "j/k Select", "enter Open")
	case viewRunDetail:
		keys = append(keys, "up/down Scroll", "home/end Jump", "enter Reload", "v Validate", "esc Runs")
	case viewPreflight:
		keys = append(keys, "p Check")
	case viewAutonomous:
		return wrapKeyLines([]string{"j/k Select", "enter Reload", "a Answer", "pgup/pgdown Scroll", "home/end Jump", "U Run Task", "Q Run Queue", "r Refresh", "1 Dashboard", "2 Tasks", "3 Runs", "4 Detail", "5 Preflight", "? Help", "q Quit"}, m.width)
	case viewDiff, viewEvidence:
		return wrapKeyLines([]string{"d Changes", "e Evidence", "A Approval", "pgup/pgdown Scroll", "home/end Jump", "r Refresh", "esc Back", "/ Commands", "q Quit"}, m.width)
	case viewApproval:
		return wrapKeyLines([]string{"a Answer", "d Changes", "e Evidence", "pgup/pgdown Scroll", "home/end Jump", "r Refresh", "esc Back", "/ Commands", "q Quit"}, m.width)
	case viewHelp:
		keys = append(keys, "esc Back")
	}
	keys = append(keys, "1 Dashboard", "2 Tasks", "3 Runs", "4 Detail", "5 Preflight", "? Help", "a Add Task", "R Run Once", fmt.Sprintf("n Passes %d", m.selectedRunLoopPasses()), "L Run Loop", "r Refresh", "q Quit")
	return wrapKeyLines(keys, m.width)
}

func (m StatusModel) commandDiscoveryLines() []string {
	commands := m.filteredSlashCommands()
	if !m.composer.DiscoveryOpen {
		return nil
	}
	width := m.contentWidth()
	if len(commands) == 0 {
		return []string{clipDisplayLine("Commands · no matching command · Esc close", width)}
	}

	selected := min(max(m.composer.SelectedCommand, 0), len(commands)-1)
	start := max(selected-maxCommandRows+1, 0)
	end := min(start+maxCommandRows, len(commands))
	lines := []string{clipDisplayLine(fmt.Sprintf("Commands %d-%d of %d · ↑/↓ select · Esc close", start+1, end, len(commands)), width)}
	for index := start; index < end; index++ {
		prefix := "  "
		if index == selected {
			prefix = "> "
		}
		command := commands[index]
		detail := command.description
		if blocker := m.commandBlocker(command.name); blocker != "" {
			detail = "[disabled] " + blocker
		}
		lines = append(lines, clipDisplayLine(prefix+command.usage+" — "+detail, width))
	}
	return lines
}

func (m StatusModel) renderDashboard() string {
	if !m.status.Initialized {
		lines := []string{"Run `revolvr init` to initialize this repository."}
		lines = appendNotice(lines, m.message)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	if m.runOnce.Started {
		return lipgloss.JoinVertical(lipgloss.Left, appendNotice(nil, m.message)...)
	}

	lines := appendNotice(nil, m.message)
	_, ok := m.latestRunHistory()
	if !ok {
		lines = append(lines, "Idle", "No runs recorded.")
		nextIndex := nextSelectedTaskIndex(m.status.Tasks)
		if nextIndex >= 0 {
			lines = append(lines, "Next task: "+taskBrief(m.status.Tasks[nextIndex]))
		} else {
			lines = append(lines, "Next task: none")
		}
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderTasks() string {
	lines := []string{"Tasks"}
	lines = appendNotice(lines, m.taskNotice())
	if !m.status.Initialized {
		lines = append(lines, "State: not initialized", "Task List", "Unavailable until state is initialized.")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	counts := countTasks(m.status.Tasks)
	nextIndex := nextSelectedTaskIndex(m.status.Tasks)
	lines = append(lines,
		fmt.Sprintf("Total: %d", counts.total),
		fmt.Sprintf("Pending: %d", counts.pending),
		fmt.Sprintf("Blocked: %d", counts.blocked),
		fmt.Sprintf("Completed: %d", counts.completed),
	)
	lines = append(lines, nextRunnableLines(m.status.Tasks, nextIndex)...)
	lines = append(lines, operatorCheckpointLines(m.status.Tasks)...)
	lines = append(lines, schedulingDiagnosticLines(m.status.Schedule.InvalidGraph)...)
	lines = append(lines, "", "Task List")
	if len(m.status.Tasks) == 0 {
		lines = append(lines,
			"No task files found.",
			"",
			"Task Detail",
			"No task selected.",
		)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	selected := clampTaskIndex(m.status.Tasks, m.selectedTaskPosition())
	for i, task := range m.status.Tasks {
		prefix := taskListPrefix(i == selected, i == nextIndex)
		summary := oneLine(task.Summary)
		if summary == "" {
			summary = oneLine(task.Task)
		}
		line := fmt.Sprintf("%s %s  %s", prefix, optionalValue(task.ID), taskListStatus(task.Status))
		if state := taskListWorkflowState(task); state != "" {
			line += "  " + state
		}
		if summary != "" {
			line += "  " + summary
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, renderTaskDetailLines(m.status.Tasks[selected])...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderRuns() string {
	lines := []string{"Runs"}
	lines = appendNotice(lines, m.runNotice())
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
	lines = appendNotice(lines, m.runNotice())
	lines = append(lines, runSummaryLines(history.Run)...)
	lines = append(lines, "")
	lines = append(lines, runTimelineLines(history)...)
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
	lines = appendNotice(lines, m.runNotice())
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

func (m StatusModel) latestRunHistory() (ledger.RunWithEvents, bool) {
	if len(m.status.RecentRuns) == 0 {
		return ledger.RunWithEvents{}, false
	}
	return ledger.RunWithEvents{Run: m.status.RecentRuns[0], Events: m.status.LatestEvents}, true
}

func (m StatusModel) focusedRunHistory() (ledger.RunWithEvents, bool) {
	if m.focusedFromAutonomous {
		return ledger.RunWithEvents{}, false
	}
	if m.focusSource == viewRunDetail && m.runDetails != nil {
		return *m.runDetails, true
	}
	if m.focusSource == viewRuns && m.selectedRun != 0 {
		if m.runDetails != nil && m.runDetails.Run.ID == m.selectedRunID() {
			return *m.runDetails, true
		}
		return ledger.RunWithEvents{}, false
	}
	return m.latestRunHistory()
}

func (m StatusModel) renderFocusedDiff() string {
	lines := []string{"Change Summary"}
	lines = appendNotice(lines, m.message)
	if m.focusedFromAutonomous {
		lines = m.appendAutonomousFocusStatus(lines)
		if m.autonomous.View == nil {
			return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No autonomous change summary loaded.")...)
		}
		view := *m.autonomous.View
		lines = append(lines,
			"Task: "+optionalValue(view.Identity.TaskID),
			"Workspace: "+optionalValue(view.Workspace.Status),
			"Source revision: "+optionalValue(view.Workspace.SourceRevision),
			"Checkpoint commit: "+optionalValue(view.Workspace.CheckpointCommit),
			"",
			"Exact Diff Artifacts",
		)
		found := false
		for _, reference := range view.Provenance.References {
			if !strings.Contains(strings.ToLower(reference.Kind), "diff") {
				continue
			}
			found = true
			lines = append(lines, fmt.Sprintf("[%s] path=%s sha256=%s bytes=%d", reference.Kind, optionalValue(reference.Path), optionalValue(reference.SHA256), reference.ByteSize))
		}
		if !found {
			lines = append(lines, "No exact diff artifact identity is present in this bounded projection.")
		}
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	history, ok := m.focusedRunHistory()
	if !ok {
		return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No canonical run projection loaded.")...)
	}
	diagnostics := runDetailDiagnosticsFromHistory(history)
	lines = append(lines, "Run: "+optionalValue(history.Run.ID), "Source: canonical changed-files events and run record", "")
	lines = append(lines, runChangedFileLines(diagnostics)...)
	commitLine := runDetailCommitLine(diagnostics)
	if commitLine == "" {
		commitLine = "commit: none"
	}
	lines = append(lines, commitLine, "", "Event Evidence")
	found := false
	for _, event := range history.Events {
		if event.Type != ledger.EventChangedFilesCaptured && event.Type != ledger.EventCommitCreated {
			continue
		}
		found = true
		lines = append(lines, fmt.Sprintf("%d  %s  %s", event.ID, event.Type, optionalTime(event.CreatedAt)))
	}
	if !found {
		lines = append(lines, "None")
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) renderFocusedEvidence() string {
	lines := []string{"Evidence"}
	lines = appendNotice(lines, m.message)
	if m.focusedFromAutonomous {
		lines = m.appendAutonomousFocusStatus(lines)
		if m.autonomous.View == nil {
			return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No autonomous evidence projection loaded.")...)
		}
		view := *m.autonomous.View
		lines = append(lines, "Task: "+optionalValue(view.Identity.TaskID), "", "Artifact Identities")
		if len(view.Provenance.References) == 0 {
			lines = append(lines, "None")
		}
		for _, reference := range view.Provenance.References {
			lines = append(lines, fmt.Sprintf("[%s] path=%s run=%s sha256=%s bytes=%d | %s", reference.Kind, optionalValue(reference.Path), optionalValue(reference.RunID), optionalValue(reference.SHA256), reference.ByteSize, oneLine(reference.Detail)))
		}
		lines = append(lines, "", "Verification Evidence")
		if len(view.Verification.Evidence) == 0 {
			lines = append(lines, "None")
		} else {
			lines = appendEvidenceReferences(lines, view.Verification.Evidence)
		}
		lines = append(lines, "", "Acceptance and Finding Evidence")
		count := len(lines)
		for _, item := range view.Acceptance {
			lines = appendEvidenceReferences(lines, item.Evidence)
			if item.Source != nil {
				lines = appendEvidenceReferences(lines, []autonomous.EvidenceReference{*item.Source})
			}
		}
		for _, finding := range view.Findings {
			lines = appendEvidenceReferences(lines, finding.Evidence)
			lines = appendEvidenceReferences(lines, finding.ResolutionEvidence)
		}
		if len(lines) == count {
			lines = append(lines, "None")
		}
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	history, ok := m.focusedRunHistory()
	if !ok {
		return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No canonical run projection loaded.")...)
	}
	lines = append(lines, "Run: "+optionalValue(history.Run.ID), "", "Artifact Identities")
	lines = append(lines, focusedRunArtifactLines(history.Events)...)
	lines = append(lines, "", "Canonical Events")
	lines = append(lines, runEventLines(history.Events)[1:]...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func appendEvidenceReferences(lines []string, references []autonomous.EvidenceReference) []string {
	for _, reference := range references {
		lines = append(lines, fmt.Sprintf("[%s] %s | %s", reference.Kind, optionalValue(reference.Reference), oneLine(reference.Detail)))
	}
	return lines
}

func focusedRunArtifactLines(events []ledger.Event) []string {
	artifacts, found := ledger.RunArtifactsFromEvents(events)
	if !found {
		return []string{"None"}
	}
	return runArtifactPathLines(artifacts, false)
}

func runArtifactPathLines(artifacts ledger.RunArtifacts, includeRequiredMissing bool) []string {
	values := []struct {
		label    string
		path     string
		required bool
	}{
		{"context payload", artifacts.ContextPayloadPath, true},
		{"context manifest", artifacts.ContextManifestPath, true},
		{"codex stdout jsonl", artifacts.CodexStdoutJSONLPath, true},
		{"codex stderr", artifacts.CodexStderrPath, true},
		{"last message", artifacts.LastMessagePath, true},
		{"receipt", artifacts.ReceiptPath, true},
		{"invalid receipt", artifacts.InvalidReceiptPath, false},
		{"dossier", artifacts.DossierPath, false},
		{"dossier manifest", artifacts.DossierManifestPath, false},
		{"supervisor dossier", artifacts.SupervisorDossierPath, false},
		{"supervisor dossier manifest", artifacts.SupervisorDossierManifestPath, false},
		{"supervisor prompt", artifacts.SupervisorPromptPath, false},
		{"supervisor schema", artifacts.SupervisorSchemaPath, false},
		{"supervisor output", artifacts.SupervisorOutputPath, false},
		{"supervisor decision", artifacts.SupervisorDecisionPath, false},
		{"supervisor provenance", artifacts.SupervisorProvenancePath, false},
		{"supervisor source", artifacts.SupervisorSourcePath, false},
		{"supervisor diagnostics", artifacts.SupervisorDiagnosticsPath, false},
		{"verification", artifacts.VerificationEvidencePath, false},
	}
	lines := []string{}
	for _, value := range values {
		path := strings.TrimSpace(value.path)
		if path != "" {
			lines = append(lines, value.label+": "+path)
		} else if includeRequiredMissing && value.required {
			lines = append(lines, value.label+": missing")
		}
	}
	if len(lines) == 0 {
		return []string{"None"}
	}
	return lines
}

func (m StatusModel) renderFocusedApproval() string {
	lines := []string{"Approval"}
	lines = appendNotice(lines, m.message)
	lines = m.appendAutonomousFocusStatus(lines)
	if m.autonomous.View == nil {
		return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No autonomous approval projection loaded.")...)
	}
	view := *m.autonomous.View
	lines = append(lines,
		"Task: "+optionalValue(view.Identity.TaskID),
		"Status: "+optionalValue(view.Identity.TaskStatus)+" | lifecycle: "+optionalValue(view.Identity.Lifecycle),
		"Latest decision: "+optionalValue(view.Why.LatestDecision),
		"Currently admitted action: "+optionalValue(view.Why.CurrentlyAdmittedAction),
		"Scheduler readiness: "+optionalValue(view.Why.SchedulerReadiness),
		"",
		"Acceptance",
	)
	if len(view.Acceptance) == 0 {
		lines = append(lines, "None")
	}
	for _, item := range view.Acceptance {
		line := fmt.Sprintf("[%s] %s: %s", item.Status, item.ID, oneLine(item.Description))
		if item.Rationale != "" {
			line += " | " + oneLine(item.Rationale)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "Operator Response", "State: "+optionalValue(view.Input.State))
	if view.Input.QuestionID == "" {
		lines = append(lines, "Question: none")
	} else {
		lines = append(lines,
			fmt.Sprintf("Question: %s | revision: %d | sha256: %s", view.Input.QuestionID, view.Input.Revision, view.Input.ContentSHA256),
			"Prompt: "+oneLine(view.Input.Question),
			"Blocking reason: "+oneLine(view.Input.BlockingReason),
		)
		for i, option := range view.Input.Options {
			prefix := " "
			if m.autonomous.Answer.Active && m.autonomous.Answer.Selected == i {
				prefix = ">"
			}
			lines = append(lines, fmt.Sprintf("%s Option %s: %s", prefix, option.ID, oneLine(option.Meaning)))
		}
		lines = append(lines, fmt.Sprintf("Recommendation (not selected): %s | %s", optionalValue(view.Input.RecommendationOption), optionalValue(view.Input.RecommendationRationale)))
	}
	if m.autonomous.Answer.Active {
		state := "choose an option"
		if m.autonomous.Answer.Confirming {
			state = "confirmation required: press enter"
		}
		lines = append(lines, "Answer control: "+state)
	}
	if m.autonomous.Answer.Result.AnswerPersisted {
		lines = append(lines, fmt.Sprintf("Last answer: id=%s option=%s persisted=true resumed=%t", optionalValue(m.autonomous.Answer.Result.AnswerID), optionalValue(m.autonomous.Answer.Result.OptionID), m.autonomous.Answer.Result.Resumed))
	}
	if m.autonomous.Answer.Err != "" {
		lines = append(lines, "Answer error: "+oneLine(m.autonomous.Answer.Err))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m StatusModel) appendAutonomousFocusStatus(lines []string) []string {
	if m.autonomous.LoadingList || m.autonomous.LoadingView {
		lines = append(lines, "Evidence status: loading")
	}
	if m.autonomous.Err != "" {
		lines = append(lines, "Evidence error: "+oneLine(m.autonomous.Err))
	}
	return lines
}

func (m StatusModel) renderPreflight() string {
	lines := []string{"Preflight"}
	lines = appendNotice(lines, m.preflightNotice())
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

func (m StatusModel) renderAutonomousWorkflow() string {
	lines := []string{"Autonomous Workflow"}
	lines = appendNotice(lines, m.workflowNotice())
	if m.autonomous.LoadingList {
		lines = append(lines, "Status: loading selectors")
	}
	lines = append(lines, "Evidence Selectors")
	if len(m.autonomous.Selectors) == 0 {
		lines = append(lines, "none")
	} else {
		selected := clampAutonomousIndex(m.autonomous.Selectors, m.selectedAutonomousPosition())
		for i, item := range m.autonomous.Selectors {
			prefix := " "
			if i == selected {
				prefix = ">"
			}
			label := fmt.Sprintf("%s [%s] %s status=%s", prefix, item.SourceKind, optionalValue(item.Label), optionalValue(item.Status))
			lines = append(lines, label)
		}
	}
	if m.autonomous.LoadingView {
		lines = append(lines, "", "Evidence status: loading "+optionalValue(m.autonomous.Selector))
	}
	if m.autonomous.Err != "" {
		lines = append(lines, "", "Evidence error: "+oneLine(m.autonomous.Err))
	}
	if m.autonomous.View == nil {
		lines = append(lines, "", "Workflow Detail", "not available")
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	v := *m.autonomous.View
	archivedAt := "none"
	if !v.Terminal.ArchivedAt.IsZero() {
		archivedAt = v.Terminal.ArchivedAt.UTC().Format(time.RFC3339Nano)
	}
	lines = append(lines, "", "Identity and lifecycle",
		fmt.Sprintf("Source: %s", v.Identity.SourceKind),
		fmt.Sprintf("Task: %s | title: %s", optionalValue(v.Identity.TaskID), optionalValue(v.Identity.Title)),
		fmt.Sprintf("Status: %s | lifecycle: %s | phase: %s", optionalValue(v.Identity.TaskStatus), optionalValue(v.Identity.Lifecycle), optionalValue(v.Summary.Phase)),
		fmt.Sprintf("Task path: %s | sha256: %s | bytes: %d", optionalValue(v.Identity.TaskPath), optionalValue(v.Identity.TaskSHA256), v.Identity.TaskByteSize),
		fmt.Sprintf("State: %s | sha256: %s | bytes: %d | schema: %s", optionalValue(v.Identity.StatePath), optionalValue(v.Identity.StateSHA256), v.Identity.StateByteSize, optionalValue(v.Identity.StateSchema)),
		fmt.Sprintf("Archive: %s | disposition: %s | verification: %s", optionalValue(v.Identity.ArchiveID), optionalValue(v.Identity.ArchiveDisposition), archiveVerificationText(v)),
	)

	currentWorker := "none"
	if v.Why.LatestDecisionReference != nil && v.Why.CurrentlyAdmittedAction != "none" {
		currentWorker = fmt.Sprintf("profile=%s run=%s decision=%s", optionalValue(string(v.Why.LatestDecisionReference.WorkerProfile)), optionalValue(v.Why.LatestDecisionReference.RunID), optionalValue(v.Why.LatestDecisionReference.DecisionID))
	}
	lines = append(lines, "", "Decision and readiness",
		"Latest decision: "+optionalValue(v.Why.LatestDecision),
		"Currently admitted action: "+optionalValue(v.Why.CurrentlyAdmittedAction),
		"Current worker: "+currentWorker,
		"Scheduler readiness: "+optionalValue(v.Why.SchedulerReadiness),
		"Next supervisor action: "+optionalValue(v.Why.NextSupervisorAction),
		"Why reasons:",
	)
	if len(v.Why.Reasons) == 0 {
		lines = append(lines, "none")
	} else {
		for _, reason := range v.Why.Reasons {
			lines = append(lines, fmt.Sprintf("- [%s] %s", reason.Code, oneLine(reason.Text)))
		}
	}

	lines = append(lines, "", "Plan", fmt.Sprintf("Progress: %d/%d", v.Summary.Plan.Completed, v.Summary.Plan.Total))
	if v.Plan == nil {
		lines = append(lines, "none")
	} else {
		lines = append(lines, fmt.Sprintf("ID: %s | revision: %d | supersedes: %s | completed: %t", v.Plan.ID, v.Plan.Revision, optionalValue(v.Plan.SupersedesPlanID), v.Plan.Completed))
		for i, step := range v.Plan.Steps {
			lines = append(lines, fmt.Sprintf("%d. [%s] %s: %s", i+1, step.Status, step.ID, oneLine(step.Description)))
			if step.Rationale != "" {
				lines = append(lines, "   rationale: "+oneLine(step.Rationale))
			}
		}
	}

	lines = append(lines, "", "Acceptance matrix", fmt.Sprintf("Progress: %d/%d", v.Summary.Acceptance.Completed, v.Summary.Acceptance.Total))
	if len(v.Acceptance) == 0 {
		lines = append(lines, "none")
	}
	for _, item := range v.Acceptance {
		line := fmt.Sprintf("[%s] %s: %s", item.Status, item.ID, oneLine(item.Description))
		if item.Rationale != "" {
			line += " | rationale: " + oneLine(item.Rationale)
		}
		lines = append(lines, line)
	}

	lines = append(lines, "", "Findings", fmt.Sprintf("Open: blocking=%d non_blocking=%d", v.Summary.OpenBlockingFindings, v.Summary.OpenNonBlockingFindings))
	if len(v.Findings) == 0 {
		lines = append(lines, "none")
	}
	for _, finding := range v.Findings {
		lines = append(lines,
			fmt.Sprintf("[%s/%s] %s: %s", finding.Status, finding.Significance, finding.ID, oneLine(finding.Summary)),
			"  correction: "+optionalValue(finding.RequiredCorrection),
			fmt.Sprintf("  introduced: revision=%d run=%s | current: revision=%d run=%s", finding.IntroducedBy.Revision, optionalValue(finding.IntroducedBy.RunID), finding.CurrentAudit.Revision, optionalValue(finding.CurrentAudit.RunID)),
		)
		if finding.ResolutionRationale != "" {
			lines = append(lines, "  resolution: "+oneLine(finding.ResolutionRationale))
		}
	}

	lines = append(lines, "", "Attempts, budgets, and worker runs",
		fmt.Sprintf("Total attempts: %d | consecutive failures: %d", v.Attempts.Total, v.Attempts.ConsecutiveFailures),
	)
	if len(v.Attempts.PerAction) == 0 {
		lines = append(lines, "Per action: none")
	}
	for _, item := range v.Attempts.PerAction {
		lines = append(lines, fmt.Sprintf("Per action: %s=%d", item.Action, item.Attempts))
	}
	for _, budget := range v.Attempts.Budgets {
		if budget.Mode == "limited" {
			lines = append(lines, fmt.Sprintf("Budget %s: limited limit=%d consumed=%d remaining=%d exhausted=%t unit=%s", budget.Name, budget.Limit, budget.Consumed, budget.Remaining, budget.Exhausted, budget.Unit))
		} else {
			lines = append(lines, fmt.Sprintf("Budget %s: %s consumed=%d unit=%s", budget.Name, optionalValue(budget.Mode), budget.Consumed, budget.Unit))
		}
	}
	if len(v.Attempts.Stops) == 0 {
		lines = append(lines, "Stops: none")
	} else {
		lines = append(lines, "Stops: "+strings.Join(v.Attempts.Stops, ","))
	}
	for _, event := range v.Attempts.Events {
		lines = append(lines, fmt.Sprintf("Attempt %d: %s kind=%s action=%s outcome=%s run=%s at=%s", event.Sequence, event.AttemptID, event.Kind, event.Action, optionalValue(event.Outcome), optionalValue(event.RunID), event.CreatedAt.UTC().Format(time.RFC3339Nano)))
	}

	lines = append(lines, "", "Operator input", "State: "+optionalValue(v.Input.State))
	if v.Input.QuestionID == "" {
		lines = append(lines, "Question: none")
	} else {
		lines = append(lines,
			fmt.Sprintf("Question: %s | revision: %d | sha256: %s", v.Input.QuestionID, v.Input.Revision, v.Input.ContentSHA256),
			"Prompt: "+oneLine(v.Input.Question),
			"Blocking reason: "+oneLine(v.Input.BlockingReason),
		)
		for i, option := range v.Input.Options {
			prefix := " "
			if m.autonomous.Answer.Active && m.autonomous.Answer.Selected == i {
				prefix = ">"
			}
			lines = append(lines, fmt.Sprintf("%s Option %s: %s", prefix, option.ID, oneLine(option.Meaning)))
		}
		lines = append(lines, fmt.Sprintf("Recommendation (not selected): %s | %s", optionalValue(v.Input.RecommendationOption), optionalValue(v.Input.RecommendationRationale)))
	}
	if m.autonomous.Answer.Active {
		state := "choose an option"
		if m.autonomous.Answer.Confirming {
			state = "confirmation required: press enter again"
		}
		lines = append(lines, "Answer control: "+state)
	}
	if m.autonomous.Answer.Result.AnswerPersisted {
		lines = append(lines, fmt.Sprintf("Last answer: id=%s option=%s persisted=true resumed=%t", optionalValue(m.autonomous.Answer.Result.AnswerID), optionalValue(m.autonomous.Answer.Result.OptionID), m.autonomous.Answer.Result.Resumed))
	}
	if m.autonomous.Answer.Err != "" {
		lines = append(lines, "Answer error: "+oneLine(m.autonomous.Answer.Err))
	}

	lines = append(lines, "", "Verification and audit",
		fmt.Sprintf("Verification: state=%s status=%s purpose=%s final_gate=%s run=%s occurrence=%s source=%s", optionalValue(v.Verification.State), optionalValue(v.Verification.Status), optionalValue(v.Verification.Purpose), optionalValue(v.Verification.FinalGate), optionalValue(v.Verification.RunID), optionalValue(v.Verification.OccurrenceID), optionalValue(v.Verification.SourceRevision)),
		fmt.Sprintf("Audit: state=%s revision=%d disposition=%s findings=%d run=%s source=%s artifact=%s", optionalValue(v.Audit.State), v.Audit.Revision, optionalValue(v.Audit.Disposition), v.Audit.FindingCount, optionalValue(v.Audit.RunID), optionalValue(v.Audit.SourceRevision), optionalValue(v.Audit.ArtifactPath)),
	)

	lines = append(lines, "", "Workspace, terminal, and archive",
		fmt.Sprintf("Workspace: state=%s id=%s status=%s root=%s branch=%s source=%s", optionalValue(v.Workspace.State), optionalValue(v.Workspace.WorkspaceID), optionalValue(v.Workspace.Status), optionalValue(v.Workspace.ExecutionRoot), optionalValue(v.Workspace.BranchRef), optionalValue(v.Workspace.SourceRevision)),
		fmt.Sprintf("Checkpoint: sequence=%d commit=%s", v.Workspace.CheckpointSequence, optionalValue(v.Workspace.CheckpointCommit)),
		fmt.Sprintf("Terminal: state=%s reason=%s finalization=%s", optionalValue(v.Terminal.State), optionalValue(v.Terminal.Reason), optionalValue(v.Terminal.FinalizationStage)),
		fmt.Sprintf("Archive: id=%s disposition=%s archived_at=%s verified_now=%t", optionalValue(v.Terminal.ArchiveID), optionalValue(v.Terminal.Disposition), archivedAt, v.Terminal.VerifiedNow),
	)

	lines = append(lines, "", "Provenance and raw references",
		"Worker runs: "+optionalJoined(v.Provenance.WorkerRunIDs),
		"Verification runs: "+optionalJoined(v.Provenance.VerificationRunIDs),
		"Audit runs: "+optionalJoined(v.Provenance.AuditRunIDs),
	)
	if len(v.Provenance.References) == 0 {
		lines = append(lines, "References: none")
	}
	for _, reference := range v.Provenance.References {
		lines = append(lines, fmt.Sprintf("Reference [%s] path=%s run=%s sha256=%s bytes=%d detail=%s", reference.Kind, optionalValue(reference.Path), optionalValue(reference.RunID), optionalValue(reference.SHA256), reference.ByteSize, oneLine(reference.Detail)))
	}
	lines = append(lines, "", "Diagnostics and omissions")
	if len(v.Diagnostics) == 0 {
		lines = append(lines, "none")
	}
	for _, diagnostic := range v.Diagnostics {
		lines = append(lines, fmt.Sprintf("[%s/%s] %s | reference=%s", diagnostic.Code, diagnostic.Section, oneLine(diagnostic.Detail), optionalValue(diagnostic.Reference)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func archiveVerificationText(view autonomousview.View) string {
	if view.Identity.SourceKind != autonomousview.SourceArchive {
		return "not applicable"
	}
	if view.Terminal.VerifiedNow {
		return "verified"
	}
	return "unverified"
}

func optionalJoined(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
}

func (m StatusModel) renderRunProgress() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.liveOperationLines()...)
}

func (m StatusModel) liveOperationLines() []string {
	if m.settling != nil {
		return slices.Clone(m.settling.cell.source)
	}

	width := m.contentWidth()
	name := optionalValue(m.runOnce.Task)
	if m.runOnce.CancelRequested {
		return []string{
			clipDisplayLine("Cancelling: "+name, width),
			"Current: waiting for the run to stop",
			"Next: wait for settlement",
		}
	}

	mode := liveOperationModeLine(m.runOnce, width)
	if elapsed := liveOperationElapsed(m.runOnce.StartedAt); elapsed != "" {
		withElapsed := mode + " · elapsed " + elapsed
		if textWidth(withElapsed) <= width {
			mode = withElapsed
		}
	}
	lines := []string{
		clipDisplayLine("Running: "+name, width),
		mode,
		"Safety: admitted",
	}
	lines = append(lines, boundedCurrentLines(m.runOnce.Current, width)...)
	return append(lines, "Next: wait, or press c or Esc to cancel")
}

func liveOperationModeLine(state runOnceState, width int) string {
	switch state.Mode {
	case runModeLoop:
		limit := max(state.Stats.MaxPasses, state.MaxPasses)
		pass := min(state.Stats.Passes+1, limit)
		return fmt.Sprintf("Mode: loop · pass %d of %d", max(pass, 1), max(limit, 1))
	case runModeTask:
		cycle := max(state.Progress, 1)
		label := "autonomous task"
		if width <= 40 {
			label = "autonomous"
		}
		return fmt.Sprintf("Mode: %s · cycle %d of %d", label, cycle, max(state.MaxPasses, 1))
	case runModeQueue:
		return fmt.Sprintf("Mode: queue · tasks %d of %d", state.Progress, max(state.MaxPasses, 1))
	default:
		return "Mode: single pass"
	}
}

func liveOperationElapsed(startedAt time.Time) string {
	if startedAt.IsZero() {
		return ""
	}
	elapsed := time.Since(startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	return elapsed.Truncate(time.Second).String()
}

func boundedCurrentLines(detail string, width int) []string {
	detail = oneLine(detail)
	if detail == "" {
		detail = "waiting for progress"
	}
	lines := wrapPlainLine("Current: "+detail, width)
	if len(lines) <= 2 {
		return lines
	}
	lines = lines[:2]
	if width <= 1 {
		lines[1] = "…"
	} else {
		prefix, _ := splitRunePrefix(lines[1], width-1)
		lines[1] = strings.TrimRight(prefix, " ") + "…"
	}
	return lines
}

func (m StatusModel) renderHelp() string {
	lines := []string{
		"Help",
		"?  Help",
		"/help  Help",
		"/tasks  Tasks",
		"/runs  Runs",
		"/detail  Run Detail",
		"Esc closes Help",
		"",
		"Views",
		"1  Dashboard",
		"2  Tasks",
		"3  Runs",
		"4  Run Detail",
		"5  Preflight",
		"6  Workflow",
		"/  Focus command composer",
		"d/e/A  Open change summary, evidence, or approval",
		"a  Add task",
		"R  Run once",
		fmt.Sprintf("n  Cycle loop max passes (current %d)", m.selectedRunLoopPasses()),
		"U  Run selected autonomous task until terminal",
		"Q  Run autonomous queue until exhausted",
		"L  Run loop",
		"r  Refresh status",
		"c  Cancel active run",
		"q  Quit",
		"",
		"Tasks: j/k Move selection, u Retry blocked selected task",
		"Runs: j/k Move selection",
		"Run Detail: up/down Scroll, home/end Jump, v Validate receipt",
		"Workflow: j/k Select, enter Reload, a Answer, pgup/pgdown Scroll",
		"Preflight: p Run readiness checks",
		"enter or o  Open selected run",
		"Run Detail: esc returns to Runs",
		"",
		"Slash Commands",
		"/dashboard /transcript /tasks /runs /detail /workflow",
		"/diff /evidence /approval /answer <option-id>",
		"/preflight /run /loop /task-run /queue",
		"/refresh /cancel /validate /help /commands /quit",
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

func countTasks(tasks []taskmodel.Task) taskCounts {
	counts := taskCounts{total: len(tasks)}
	for _, task := range tasks {
		switch task.Status {
		case taskmodel.StatusPending:
			counts.pending++
		case taskmodel.StatusBlocked:
			counts.blocked++
		case taskmodel.StatusCompleted:
			counts.completed++
		}
	}
	return counts
}

func nextSelectedTaskIndex(tasks []taskmodel.Task) int {
	for i, task := range tasks {
		if task.Status == taskmodel.StatusPending && task.NextRunnable {
			return i
		}
	}
	for i, task := range tasks {
		if task.Status == taskmodel.StatusPending && task.NextAutonomous {
			return i
		}
	}
	return -1
}

func nextRunnableLines(tasks []taskmodel.Task, nextIndex int) []string {
	if nextIndex < 0 || nextIndex >= len(tasks) {
		return []string{
			"Runnable: nothing runnable",
			"Next task: none",
		}
	}
	lines := []string{
		"Runnable: ready to run",
		"Next task: " + taskBrief(tasks[nextIndex]),
	}
	if state := taskWorkflowStateLine(tasks[nextIndex]); state != "" {
		lines = append(lines, state)
	}
	return lines
}

func taskBrief(task taskmodel.Task) string {
	id := optionalValue(task.ID)
	summary := oneLine(task.Summary)
	if summary == "" {
		summary = oneLine(task.Task)
	}
	if summary == "" {
		return id
	}
	return fmt.Sprintf("%s - %s", id, summary)
}

func liveTaskName(task taskmodel.Task) string {
	if name := oneLine(task.Summary); name != "" {
		return name
	}
	if name := oneLine(task.Task); name != "" {
		return name
	}
	return strings.TrimSpace(task.ID)
}

func liveRunResultName(result runonce.Result) string {
	if name := oneLine(result.Run.Task); name != "" {
		return name
	}
	return liveTaskName(result.Task)
}

func (m StatusModel) nextLiveTaskName() string {
	index := nextSelectedTaskIndex(m.status.Tasks)
	if index < 0 {
		return "run"
	}
	return liveTaskName(m.status.Tasks[index])
}

func renderTaskDetailLines(task taskmodel.Task) []string {
	lines := []string{
		"Task Detail",
		fmt.Sprintf("ID: %s", optionalValue(task.ID)),
		fmt.Sprintf("Status: %s", optionalValue(task.Status)),
	}
	if hasTaskWorkflowState(task) {
		lines = append(lines, fmt.Sprintf("Workflow: %s", optionalValue(task.Workflow)))
		if task.Workflow == taskfile.WorkflowOperatorCheckpointV1 {
			lines = append(lines,
				fmt.Sprintf("Checkpoint: %s", optionalValue(task.CheckpointState)),
				fmt.Sprintf("Checkpoint receipt: %s", optionalValue(task.CheckpointReceiptPath)),
				fmt.Sprintf("Checkpoint receipt SHA-256: %s", optionalValue(task.CheckpointReceiptSHA)),
			)
		} else {
			lines = append(lines,
				fmt.Sprintf("Phase: %s", optionalValue(task.Phase)),
				fmt.Sprintf("Profile: %s", optionalValue(task.RunProfile)),
				fmt.Sprintf("Next: %s", optionalValue(task.NextState)),
			)
		}
	}
	lines = append(lines,
		fmt.Sprintf("Readiness: %s", optionalValue(task.ReadinessReason)),
		fmt.Sprintf("Waiting on: %s", optionalValue(strings.Join(task.WaitingDependencyIDs, ","))),
		fmt.Sprintf("Depends on: %s", optionalValue(strings.Join(task.DependsOn, ","))),
		fmt.Sprintf("Tags: %s", optionalValue(strings.Join(task.Tags, ","))),
		fmt.Sprintf("Conflicts: %s", optionalValue(strings.Join(task.Conflicts, ","))),
		fmt.Sprintf("Conflict blockers: %s", optionalValue(strings.Join(task.ConflictBlockers, ","))),
		fmt.Sprintf("Parent: %s", optionalValue(task.ParentTaskID)),
	)
	for _, diagnostic := range task.SchedulingDiagnostics {
		lines = append(lines, fmt.Sprintf("Scheduling diagnostic: %s: %s", diagnostic.Code, oneLine(diagnostic.Detail)))
	}
	lines = append(lines,
		fmt.Sprintf("Summary: %s", optionalValue(task.Summary)),
		fmt.Sprintf("Task: %s", optionalValue(task.Task)),
		fmt.Sprintf("Blocker: %s", optionalValue(task.Blocker)),
	)
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

func schedulingDiagnosticLines(diagnostics []taskscheduler.Diagnostic) []string {
	lines := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		lines = append(lines, fmt.Sprintf("Scheduling diagnostic: %s: %s", diagnostic.Code, oneLine(diagnostic.Detail)))
	}
	return lines
}

func operatorCheckpointLines(tasks []taskmodel.Task) []string {
	lines := make([]string, 0)
	for _, task := range tasks {
		if task.Workflow != taskfile.WorkflowOperatorCheckpointV1 {
			continue
		}
		lines = append(lines, fmt.Sprintf("Operator checkpoint: %s  state=%s  receipt=%s",
			optionalValue(task.ID),
			optionalValue(task.CheckpointState),
			optionalValue(task.CheckpointReceiptPath),
		))
	}
	return lines
}

func hasTaskWorkflowState(task taskmodel.Task) bool {
	return strings.TrimSpace(task.Workflow) != "" ||
		strings.TrimSpace(task.Phase) != "" ||
		strings.TrimSpace(task.RunProfile) != "" ||
		strings.TrimSpace(task.NextState) != ""
}

func taskWorkflowStateLine(task taskmodel.Task) string {
	if !hasTaskWorkflowState(task) {
		return ""
	}
	return fmt.Sprintf("Workflow: %s  Phase: %s  Profile: %s  Next: %s",
		optionalValue(task.Workflow),
		optionalValue(task.Phase),
		optionalValue(task.RunProfile),
		optionalValue(task.NextState),
	)
}

func taskListWorkflowState(task taskmodel.Task) string {
	if !hasTaskWorkflowState(task) {
		return ""
	}
	if task.Workflow == taskfile.WorkflowOperatorCheckpointV1 {
		return fmt.Sprintf("checkpoint=%s  receipt=%s",
			optionalValue(task.CheckpointState),
			optionalValue(task.CheckpointReceiptPath),
		)
	}
	return fmt.Sprintf("phase=%s  profile=%s  next=%s",
		optionalValue(task.Phase),
		optionalValue(task.RunProfile),
		optionalValue(task.NextState),
	)
}

func taskListPrefix(selected bool, nextRunnable bool) string {
	prefix := " "
	if selected {
		prefix = ">"
	}
	if nextRunnable {
		return prefix + " next"
	}
	return prefix + " -"
}

func taskListStatus(status string) string {
	switch status {
	case taskmodel.StatusBlocked:
		return "! blocked"
	case "":
		return "none"
	default:
		return status
	}
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
	selected := clampRunIndex(m.status.RecentRuns, m.selectedRunPosition())
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

func runTimelineLines(history ledger.RunWithEvents) []string {
	rows := app.RunTimeline(history)
	lines := []string{"Timeline"}
	if len(rows) == 0 {
		return append(lines, "No timeline rows.")
	}
	lines = append(lines, "TIMESTAMP  PHASE  STATUS  DETAIL")
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf(
			"%s  %s  %s  %s",
			optionalTime(row.Timestamp),
			optionalValue(row.Phase),
			optionalValue(row.Status),
			optionalValue(row.Detail),
		))
	}
	return lines
}

func dashboardTimelineLine(row app.RunTimelineRow) string {
	label := strings.TrimSpace(row.Phase + " " + row.Status)
	detail := oneLine(row.Detail)
	switch {
	case detail == "from run record",
		row.Phase == "codex" && row.Status == "started" && detail == "codex":
		detail = ""
	case row.Phase == "codex" && row.Status == "progress" && strings.HasPrefix(detail, "message: "):
		label = "codex"
		detail = strings.TrimPrefix(detail, "message: ")
	case (row.Phase == "changes" && row.Status == "captured") ||
		(row.Phase == "commit" && row.Status == "started"):
		detail, _, _ = strings.Cut(detail, ": ")
	case row.Phase == "commit" && row.Status == "created":
		detail = strings.TrimPrefix(detail, "commit ")
		if len(detail) > 12 {
			detail = detail[:12]
		}
	case row.Phase == "receipt" && row.Status == "parsed":
		detail, _, _ = strings.Cut(detail, " (")
	case row.Phase == "receipt" && row.Status == "warning":
		label = "receipt"
		if _, message, ok := strings.Cut(detail, ": "); ok {
			detail = message
		}
		detail = strings.ReplaceAll(detail, "harness ", "")
		detail = strings.TrimPrefix(detail, "receipt ")
		detail, _, _ = strings.Cut(detail, " (")
		detail = strings.ReplaceAll(detail, "captured changed files", "captured files")
	}
	line := fmt.Sprintf("%s %s  %s", dashboardTimelineMarker(row.Status), dashboardTimelineTime(row.Timestamp), label)
	if detail != "" {
		line += " — " + truncateDashboardDetail(detail)
	}
	return line
}

func dashboardTimelineMarker(status string) string {
	switch status {
	case "completed", "passed", "created", "captured":
		return "✓"
	case "failed", "error", "cancelled":
		return "×"
	case "warning":
		return "!"
	case "selected", "progress":
		return "›"
	default:
		return "•"
	}
}

func dashboardTimelineTime(value time.Time) string {
	if value.IsZero() {
		return "--:--"
	}
	return value.UTC().Format("15:04")
}

func truncateDashboardDetail(value string) string {
	runes := []rune(value)
	if len(runes) <= maxDashboardDetail {
		return value
	}
	return string(runes[:maxDashboardDetail-1]) + "…"
}

func runArtifactLines(events []ledger.Event) []string {
	lines := []string{"Artifacts"}
	artifacts, found := ledger.RunArtifactsFromEvents(events)
	if !found {
		return append(lines, "No artifact event found.")
	}
	return append(lines, runArtifactPathLines(artifacts, true)...)
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

func taskProgressDetail(operation autonomoustaskrun.Operation) string {
	stage := strings.ReplaceAll(strings.TrimSpace(operation.Stage), "_", " ")
	action := strings.ReplaceAll(strings.TrimSpace(operation.LastAction), "_", " ")
	switch {
	case action != "" && stage != "":
		return action + " · " + stage
	case action != "":
		return action
	case stage != "":
		return stage
	default:
		return "waiting for task progress"
	}
}

func queueProgressDetail(operation autonomousqueue.Operation) string {
	parts := []string{strings.ReplaceAll(strings.TrimSpace(operation.Stage), "_", " ")}
	if operation.InFlight != nil {
		parts = append(parts, "task "+operation.InFlight.TaskID)
	} else {
		for _, slot := range operation.Slots {
			parts = append(parts, "task "+slot.Selection.TaskID)
		}
	}
	parts = compactRunDetailStrings(parts)
	if len(parts) == 0 {
		return "waiting for queue progress"
	}
	return strings.Join(parts, " · ")
}

func runPassSummaryLine(pass int, result runonce.Result) string {
	return fmt.Sprintf("pass %d: %s", pass, runResultSummary(result))
}

func runResultSummary(result runonce.Result) string {
	if result.NoTask || result.Outcome == runonce.OutcomeNoTask {
		return "no pending runnable tasks"
	}
	runID := optionalValue(result.Run.ID)
	taskID := optionalValue(result.Task.ID)
	switch result.Outcome {
	case runonce.OutcomeCommitted:
		return fmt.Sprintf("run %s completed task %s; commit %s", runID, taskID, optionalValue(result.Commit.CommitSHA))
	default:
		message := oneLine(result.Message)
		if message == "" {
			message = "no message"
		}
		return fmt.Sprintf("run %s stopped (%s): %s", runID, optionalValue(string(result.Outcome)), message)
	}
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

func runLoopTerminalStatus(stats app.RunLoopStats, err error, cancelled bool) string {
	switch {
	case cancelled:
		return "cancelled"
	case stats.StopReason == "no_task":
		return "no_task"
	case err != nil:
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
				if current != "" {
					current = ""
					continue
				}
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

func styleFooterLines(lines []string) []string {
	styled := append([]string(nil), lines...)
	for i, line := range styled {
		if strings.HasPrefix(strings.TrimSpace(line), "›") || strings.HasPrefix(line, "> ") {
			styled[i] = selectedStyle.Render(line)
		} else {
			styled[i] = mutedStyle.Render(line)
		}
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
	if strings.HasPrefix(trimmed, "Notice:") || strings.HasPrefix(trimmed, "Warning:") {
		return warningStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "FAIL ") ||
		strings.HasPrefix(trimmed, "Error:") ||
		strings.HasPrefix(trimmed, "× ") ||
		strings.Contains(trimmed, "! blocked") ||
		lineStatusIn(trimmed, "failed", "error", "blocked", "cancelled") {
		return dangerStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "OK ") ||
		strings.HasPrefix(trimmed, "PASS ") ||
		strings.HasPrefix(trimmed, "✓ ") ||
		lineStatusIn(trimmed, "ready", "passed", "completed", "initialized") {
		return successStyle.Render(line)
	}
	if lineStatusIn(trimmed, "not run", "running", "not initialized") ||
		strings.HasPrefix(trimmed, "• ") ||
		strings.HasPrefix(trimmed, "! ") ||
		strings.HasPrefix(trimmed, "Cancellation:") ||
		strings.HasPrefix(trimmed, "Capture error:") {
		return warningStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, ">") || strings.HasPrefix(trimmed, "› ") {
		return selectedStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "… ") {
		return mutedStyle.Render(line)
	}
	return line
}

func isSectionHeading(value string) bool {
	switch value {
	case "Dashboard",
		"Idle",
		"Transcript",
		"Tasks",
		"Task List",
		"Task Detail",
		"Latest Run",
		"Recent Runs",
		"Runs",
		"Run Detail",
		"Summary",
		"Timeline",
		"Activity",
		"Diagnostics",
		"Receipt Validation",
		"Changed Files",
		"Artifacts",
		"Events",
		"Canonical Events",
		"Change Summary",
		"Exact Diff Artifacts",
		"Event Evidence",
		"Evidence",
		"Artifact Identities",
		"Verification Evidence",
		"Acceptance and Finding Evidence",
		"Approval",
		"Acceptance",
		"Operator Response",
		"Preflight",
		"Run Progress",
		"Log",
		"Help",
		"Views",
		"Slash Commands",
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
		if slices.Contains(statuses, value) {
			return true
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
	width := 0
	for i, r := range value {
		if width+textWidth(string(r)) > n {
			return value[:i], value[i:]
		}
		width += textWidth(string(r))
	}
	return value, ""
}

func clipDisplayLine(value string, width int) string {
	if textWidth(value) <= width {
		return value
	}
	if width <= 1 {
		prefix, _ := splitRunePrefix(value, max(width, 1))
		return prefix
	}
	prefix, _ := splitRunePrefix(value, width-1)
	return prefix + "…"
}

func textWidth(value string) int {
	return lipgloss.Width(value)
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
		m.setSelectedTask(0)
		return
	}
	m.setSelectedTask(m.selectedTaskPosition() + delta)
}

func (m StatusModel) tasksOverlayActive() bool {
	return m.overlay != nil && (m.overlay.content == viewTasks || m.overlay.content == viewTaskEntry)
}

func (m StatusModel) selectedTaskPosition() int {
	if m.tasksOverlayActive() {
		return m.overlay.selected
	}
	return m.selectedTask
}

func (m *StatusModel) setSelectedTask(index int) {
	index = clampTaskIndex(m.status.Tasks, index)
	if m.tasksOverlayActive() {
		m.overlay.selected = index
		return
	}
	m.selectedTask = index
}

func (m StatusModel) taskNotice() string {
	if m.tasksOverlayActive() {
		return m.overlay.message
	}
	return m.message
}

func (m *StatusModel) setTaskNotice(message string) {
	if m.tasksOverlayActive() {
		m.overlay.message = message
		return
	}
	m.message = message
}

func (m *StatusModel) setRefreshNotice(message string) {
	if m.tasksOverlayActive() || m.runsOverlayActive() || m.preflightOverlayActive() || m.workflowOverlayActive() {
		m.overlay.message = message
		return
	}
	m.message = message
}

func (m StatusModel) preflightOverlayActive() bool {
	return m.overlay != nil && m.overlay.content == viewPreflight
}

func (m StatusModel) preflightNotice() string {
	if m.preflightOverlayActive() {
		return m.overlay.message
	}
	return m.message
}

func (m *StatusModel) setPreflightNotice(message string) {
	if m.preflightOverlayActive() {
		m.overlay.message = message
		return
	}
	m.message = message
}

func (m StatusModel) workflowOverlayActive() bool {
	return m.overlay != nil && m.overlay.content == viewAutonomous
}

func (m StatusModel) workflowActive() bool {
	return m.view == viewAutonomous || m.workflowOverlayActive()
}

func (m StatusModel) selectedAutonomousPosition() int {
	if m.workflowOverlayActive() {
		return m.overlay.selected
	}
	return m.autonomous.Selected
}

func (m *StatusModel) setSelectedAutonomous(index int) {
	if m.workflowOverlayActive() {
		m.overlay.selected = index
		return
	}
	m.autonomous.Selected = index
}

func (m StatusModel) workflowNotice() string {
	if m.workflowOverlayActive() {
		return m.overlay.message
	}
	return m.message
}

func (m *StatusModel) setWorkflowNotice(message string) {
	if m.workflowOverlayActive() {
		m.overlay.message = message
		return
	}
	m.message = message
}

func (m StatusModel) runsOverlayActive() bool {
	return m.overlay != nil && (m.overlay.content == viewRuns || m.overlay.content == viewRunDetail)
}

func (m StatusModel) selectedRunPosition() int {
	if m.runsOverlayActive() {
		return m.overlay.selected
	}
	return m.selectedRun
}

func (m *StatusModel) setSelectedRun(index int) {
	index = clampRunIndex(m.status.RecentRuns, index)
	if m.runsOverlayActive() {
		m.overlay.selected = index
		return
	}
	m.selectedRun = index
}

func (m StatusModel) runNotice() string {
	if m.runsOverlayActive() {
		return m.overlay.message
	}
	return m.message
}

func (m *StatusModel) setRunNotice(message string) {
	if m.runsOverlayActive() {
		m.overlay.message = message
		return
	}
	m.message = message
}

func (m StatusModel) selectedTaskID() string {
	if len(m.status.Tasks) == 0 {
		return ""
	}
	return strings.TrimSpace(m.status.Tasks[clampTaskIndex(m.status.Tasks, m.selectedTaskPosition())].ID)
}

func (m StatusModel) selectedTaskValue() (taskmodel.Task, bool) {
	if len(m.status.Tasks) == 0 {
		return taskmodel.Task{}, false
	}
	return m.status.Tasks[clampTaskIndex(m.status.Tasks, m.selectedTaskPosition())], true
}

func (m StatusModel) retrySelectedTaskAvailable() bool {
	task, ok := m.selectedTaskValue()
	return ok &&
		task.Status == taskmodel.StatusBlocked &&
		m.actions.RetryTask != nil &&
		m.actions.RefreshStatus != nil
}

func (m *StatusModel) moveSelectedRun(delta int) {
	if len(m.status.RecentRuns) == 0 {
		m.setSelectedRun(0)
		return
	}
	m.setSelectedRun(m.selectedRunPosition() + delta)
}

func (m StatusModel) selectedRunID() string {
	if len(m.status.RecentRuns) == 0 {
		return ""
	}
	return strings.TrimSpace(m.status.RecentRuns[clampRunIndex(m.status.RecentRuns, m.selectedRunPosition())].ID)
}

func selectedTaskIndex(tasks []taskmodel.Task, taskID string) int {
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

func clampTaskIndex(tasks []taskmodel.Task, index int) int {
	if len(tasks) == 0 || index < 0 {
		return 0
	}
	if index >= len(tasks) {
		return len(tasks) - 1
	}
	return index
}

func clampAutonomousIndex(values []app.AutonomousTaskSelector, index int) int {
	if len(values) == 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= len(values) {
		return len(values) - 1
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
