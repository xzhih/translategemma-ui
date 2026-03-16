package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/runtime"
	"translategemma-ui/internal/runtimeutil"
	"translategemma-ui/internal/translate"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.applyLayout()
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.activity, cmd = m.activity.Update(msg)
		if m.shouldAnimateActivity() {
			return m, cmd
		}
		return m, nil
	case progress.FrameMsg:
		var cmds []tea.Cmd
		nextDownload, downloadCmd := m.downloadBar.Update(msg)
		m.downloadBar = nextDownload.(progress.Model)
		if downloadCmd != nil {
			cmds = append(cmds, downloadCmd)
		}
		nextLoad, loadCmd := m.loadBar.Update(msg)
		m.loadBar = nextLoad.(progress.Model)
		if loadCmd != nil {
			cmds = append(cmds, loadCmd)
		}
		return m, tea.Batch(cmds...)
	case taskStartedMsg:
		return m.handleTaskStarted(msg)
	case taskClosedMsg:
		return m.handleTaskClosed(msg), nil
	case streamDeltaMsg:
		m.output += msg.Delta
		m.syncOutputViewport()
		return m, waitForTaskCmd(streamTaskKind, m.streamTask)
	case streamDoneMsg:
		if msg.Final != "" {
			m.output = msg.Final
		}
		m.runtimeReady = true
		m.streaming = false
		m.releaseTask(streamTaskKind)
		_ = m.syncCatalogList()
		m.status = "Translation completed"
		m.syncOutputViewport()
		return m, nil
	case streamErrMsg:
		m.runtimeReady = runtime.ProbeBackend(m.backendURL).Ready
		m.streaming = false
		m.releaseTask(streamTaskKind)
		_ = m.syncCatalogList()
		m.status = msg.Message
		return m, nil
	case provisionProgressMsg:
		cmds := []tea.Cmd{waitForTaskCmd(provisionTaskKind, m.workTask)}
		m.provisionStage = msg.Stage
		if msg.Stage == "download" {
			m.downloadPercent = msg.Percent
			if msg.Percent >= 0 {
				cmds = append(cmds, m.downloadBar.SetPercent(msg.Percent/100))
			}
		}
		if msg.Stage == "load" {
			m.loadPercent = msg.Percent
			if msg.Percent >= 0 {
				cmds = append(cmds, m.loadBar.SetPercent(msg.Percent/100))
			}
		}
		if msg.Message != "" {
			m.status = msg.Message
		}
		return m, tea.Batch(cmds...)
	case provisionDoneMsg:
		m.syncBackendURL(msg.BackendURL, false)
		m.runtimeReady = true
		m.screen = translateScreen
		m.provisionStage = ""
		m.downloadPercent = 100
		m.loadPercent = 100
		m.status = msg.Message
		m.releaseTask(provisionTaskKind)
		m.focus = textFocus
		if msg.ModelPath != "" {
			m.state.ActiveModelPath = msg.ModelPath
			_ = config.SaveAppState(m.dataRoot, m.state)
		}
		_ = m.syncCatalogList()
		focusCmd := m.setFocus(textFocus)
		if m.pendingRequest == nil {
			return m, focusCmd
		}
		req := *m.pendingRequest
		m.pendingRequest = nil
		m.output = ""
		m.syncOutputViewport()
		m.status = "Streaming translation..."
		m.streaming = true
		return m, tea.Batch(focusCmd, startStreamCmd(req, m.service), m.activity.Tick)
	case provisionErrMsg:
		m.runtimeReady = runtime.ProbeBackend(m.backendURL).Ready
		if m.pendingRequest != nil {
			m.screen = translateScreen
		} else {
			m.screen = modelScreen
		}
		m.provisionStage = ""
		m.downloadPercent = -1
		m.loadPercent = -1
		m.pendingRequest = nil
		m.cancelTask(provisionTaskKind)
		_ = m.syncCatalogList()
		m.status = msg.Message
		return m, nil
	case tea.KeyMsg:
		next, cmd, handled := m.handleKey(msg)
		if handled {
			return next, cmd
		}
		typedNext, ok := next.(model)
		if !ok {
			return next, cmd
		}
		m = typedNext
	}

	if m.screen == translateScreen {
		return m.updateFocusedComponent(msg)
	}
	return m, nil
}

func (m model) handleTaskStarted(msg taskStartedMsg) (tea.Model, tea.Cmd) {
	switch msg.kind {
	case streamTaskKind:
		m.cancelTask(streamTaskKind)
		m.streamTask = msg.task
	case provisionTaskKind:
		m.cancelTask(provisionTaskKind)
		m.workTask = msg.task
	}
	_ = m.syncCatalogList()
	return m, waitForTaskCmd(msg.kind, msg.task)
}

func (m model) handleTaskClosed(msg taskClosedMsg) model {
	switch msg.kind {
	case streamTaskKind:
		if msg.task != nil && m.streamTask != msg.task {
			return m
		}
		m.releaseTask(streamTaskKind)
		m.streaming = false
	case provisionTaskKind:
		if msg.task != nil && m.workTask != msg.task {
			return m
		}
		m.releaseTask(provisionTaskKind)
		m.pendingRequest = nil
	}
	return m
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if key.Matches(msg, m.keys.Quit) {
		m.cancelAllTasks()
		return m, tea.Quit, true
	}

	switch m.screen {
	case modelScreen:
		next, cmd := m.updateModelScreen(msg)
		return next, cmd, true
	case provisionScreen:
		next, cmd := m.updateProvisionScreen(msg)
		return next, cmd, true
	case translateScreen:
		return m.updateTranslateScreen(msg)
	default:
		return m, nil, false
	}
}

func (m model) updateModelScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.QuitAlt):
		m.cancelAllTasks()
		return m, tea.Quit
	case key.Matches(msg, m.keys.Confirm):
		if len(m.catalog) == 0 {
			m.status = "No model options available"
			return m, nil
		}
		m.cursor = m.modelList.Index()
		if m.cursor < 0 || m.cursor >= len(m.catalog) {
			m.cursor = clamp(m.cursor, 0, len(m.catalog)-1)
		}
		selectedItem := m.catalog[m.cursor]
		selected := selectedItem.QuantizedModel
		m.selected = &selected
		m.selectedName = selected.FileName
		m.cfg.ActiveModelID = selected.ID
		_ = config.SaveAppConfig(m.dataRoot, m.cfg)
		m.syncBackendURL(m.runtime.CurrentBackendURL(), false)
		probe := runtime.ProbeBackend(m.backendURL)
		m.runtimeReady = probe.Ready
		_ = m.syncCatalogList()
		if selectedItem.Installed && runtimeutil.CanReuseLoadedRuntime(selectedItem.Path, m.state.ActiveModelPath, probe.Ready) {
			m.screen = translateScreen
			m.provisionStage = ""
			m.downloadPercent = -1
			m.loadPercent = -1
			focusCmd := m.setFocus(textFocus)
			m.status = fmt.Sprintf("Runtime already loaded: %s", selected.FileName)
			return m, focusCmd
		}

		m.screen = provisionScreen
		m.provisionStage = ""
		m.downloadPercent = -1
		m.loadPercent = 0
		if localPath := m.localModelPath(selected.FileName); localPath != "" {
			m.provisionStage = "load"
			m.status = fmt.Sprintf("Loading local runtime: %s", selected.FileName)
			m.runtimeReady = false
			return m, tea.Batch(startActivateRuntimeCmd(m.dataRoot, m.runtime, m.state, localPath), m.activity.Tick)
		}

		m.status = "Installing selected model..."
		m.downloadPercent = 0
		m.loadPercent = 0
		m.runtimeReady = false
		return m, tea.Batch(startInstallModelCmd(m.dataRoot, selected, m.runtime, m.state), m.activity.Tick)
	}

	var cmd tea.Cmd
	previous := m.modelList.Index()
	m.modelList, cmd = m.modelList.Update(msg)
	if current := m.modelList.Index(); current != previous {
		m.cursor = current
		if item, ok := m.currentCatalogItem(); ok {
			if item.Installed {
				m.status = fmt.Sprintf("Ready locally: %s", item.FileName)
			} else {
				m.status = fmt.Sprintf("Download required: %s", item.FileName)
			}
		}
	}
	return m, cmd
}

func (m model) updateProvisionScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.QuitAlt) {
		m.cancelAllTasks()
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateTranslateScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.cancelTask(streamTaskKind)
		m.streaming = false
		m.screen = modelScreen
		m.status = "Back to model selection"
		m.input.Blur()
		m.instruction.Blur()
		return m, m.syncCatalogList(), true
	case key.Matches(msg, m.keys.Swap):
		m.sourceLang, m.targetLang = m.targetLang, m.sourceLang
		m.status = fmt.Sprintf("Languages swapped: %s -> %s", languages.Label(m.sourceLang), languages.Label(m.targetLang))
		return m, nil, true
	case key.Matches(msg, m.keys.Run):
		if m.streaming {
			return m, nil, true
		}
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			m.status = "Input text is empty"
			return m, nil, true
		}
		m.syncBackendURL(m.runtime.CurrentBackendURL(), false)
		probe := runtime.ProbeBackend(m.backendURL)
		m.runtimeReady = probe.Ready
		streamReq := translate.Request{
			SourceLang:             m.sourceLang,
			TargetLang:             m.targetLang,
			Text:                   text,
			TranslationInstruction: strings.TrimSpace(m.instruction.Value()),
		}
		if !probe.Ready {
			modelPath := m.resolveActiveRuntimePath()
			if modelPath == "" {
				m.status = "No local runtime detected. Choose a model to install."
				return m, nil, true
			}
			req := streamReq
			m.pendingRequest = &req
			m.screen = provisionScreen
			m.provisionStage = "load"
			m.downloadPercent = -1
			m.loadPercent = 0
			m.output = ""
			m.syncOutputViewport()
			m.status = fmt.Sprintf("Loading selected runtime: %s", m.currentRuntimeName())
			return m, tea.Batch(startActivateRuntimeCmd(m.dataRoot, m.runtime, m.state, modelPath), m.activity.Tick), true
		}
		m.cancelTask(streamTaskKind)
		m.output = ""
		m.syncOutputViewport()
		m.status = "Streaming translation..."
		m.streaming = true
		return m, tea.Batch(startStreamCmd(streamReq, m.service), m.activity.Tick), true
	case key.Matches(msg, m.keys.FocusNext):
		return m, m.rotateFocus(1), true
	case key.Matches(msg, m.keys.FocusPrev):
		return m, m.rotateFocus(-1), true
	case key.Matches(msg, m.keys.MoveUp):
		if m.focus == sourceFocus || m.focus == targetFocus {
			m.changeFocusedLanguage(-1)
			return m, nil, true
		}
		if m.focus == outputFocus {
			m.outputView.LineUp(1)
			return m, nil, true
		}
	case key.Matches(msg, m.keys.MoveDown):
		if m.focus == sourceFocus || m.focus == targetFocus {
			m.changeFocusedLanguage(1)
			return m, nil, true
		}
		if m.focus == outputFocus {
			m.outputView.LineDown(1)
			return m, nil, true
		}
	case key.Matches(msg, m.keys.Top):
		if m.focus == outputFocus {
			m.outputView.GotoTop()
			return m, nil, true
		}
	case key.Matches(msg, m.keys.Bottom):
		if m.focus == outputFocus {
			m.outputView.GotoBottom()
			return m, nil, true
		}
	case key.Matches(msg, m.keys.PageUp):
		if m.focus == outputFocus {
			m.outputView.PageUp()
			return m, nil, true
		}
	case key.Matches(msg, m.keys.PageDown):
		if m.focus == outputFocus {
			m.outputView.PageDown()
			return m, nil, true
		}
	}
	return m, nil, false
}

func (m model) updateFocusedComponent(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.focus == sourceFocus || m.focus == targetFocus || m.focus == outputFocus {
		return m, nil
	}
	var cmd tea.Cmd
	if m.focus == instructionFocus {
		m.instruction, cmd = m.instruction.Update(msg)
		return m, cmd
	}
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *model) cancelTask(kind taskKind) {
	switch kind {
	case streamTaskKind:
		if m.streamTask != nil {
			m.streamTask.stop()
			m.streamTask = nil
		}
	case provisionTaskKind:
		if m.workTask != nil {
			m.workTask.stop()
			m.workTask = nil
		}
	}
}

func (m *model) releaseTask(kind taskKind) {
	switch kind {
	case streamTaskKind:
		m.streamTask = nil
	case provisionTaskKind:
		m.workTask = nil
	}
}

func (m *model) cancelAllTasks() {
	m.cancelTask(streamTaskKind)
	m.cancelTask(provisionTaskKind)
}

func (m model) shouldAnimateActivity() bool {
	return m.screen == provisionScreen || m.streaming
}
