package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/runtime"
	"translategemma-ui/internal/translate"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.applyLayout()
		return m, nil
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
		m.streaming = false
		m.cancelTask(streamTaskKind)
		m.status = "Translation completed"
		m.syncOutputViewport()
		return m, nil
	case streamErrMsg:
		m.streaming = false
		m.cancelTask(streamTaskKind)
		m.status = msg.Message
		return m, nil
	case provisionProgressMsg:
		m.provisionStage = msg.Stage
		if msg.Stage == "download" {
			m.downloadPercent = msg.Percent
		}
		if msg.Stage == "load" {
			m.loadPercent = msg.Percent
		}
		if msg.Message != "" {
			m.status = msg.Message
		}
		return m, waitForTaskCmd(provisionTaskKind, m.workTask)
	case provisionDoneMsg:
		m.screen = translateScreen
		m.provisionStage = ""
		m.downloadPercent = 100
		m.loadPercent = 100
		m.status = msg.Message
		m.startupRuntimePath = ""
		m.cancelTask(provisionTaskKind)
		m.focus = textFocus
		if msg.ModelPath != "" {
			m.state.ActiveModelPath = msg.ModelPath
			_ = config.SaveAppState(m.dataRoot, m.state)
		}
		return m, m.setFocus(textFocus)
	case provisionErrMsg:
		m.screen = modelScreen
		m.provisionStage = ""
		m.downloadPercent = -1
		m.loadPercent = -1
		m.startupRuntimePath = ""
		m.cancelTask(provisionTaskKind)
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
	return m, waitForTaskCmd(msg.kind, msg.task)
}

func (m model) handleTaskClosed(msg taskClosedMsg) model {
	switch msg.kind {
	case streamTaskKind:
		m.cancelTask(streamTaskKind)
		m.streaming = false
	case provisionTaskKind:
		m.cancelTask(provisionTaskKind)
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
	case key.Matches(msg, m.keys.MoveUp):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.MoveDown):
		if m.cursor < len(m.models)-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.Confirm):
		if len(m.models) == 0 {
			m.status = "No model options available"
			return m, nil
		}
		selected := m.models[m.cursor]
		m.selected = &selected
		m.selectedName = selected.FileName
		m.cfg.ActiveModelID = selected.ID
		_ = config.SaveAppConfig(m.dataRoot, m.cfg)

		m.screen = provisionScreen
		m.provisionStage = ""
		m.downloadPercent = -1
		m.loadPercent = 0
		if localPath := m.localModelPath(selected.FileName); localPath != "" {
			m.provisionStage = "load"
			m.status = fmt.Sprintf("Loading local runtime: %s", selected.FileName)
			m.startupRuntimePath = ""
			return m, startActivateRuntimeCmd(m.dataRoot, m.runtime, m.state, localPath)
		}

		m.status = "Installing selected model..."
		m.downloadPercent = 0
		m.loadPercent = 0
		return m, startInstallModelCmd(m.dataRoot, selected, m.runtime, m.state)
	}
	return m, nil
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
		return m, nil, true
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
		if probe := runtime.ProbeBackend(m.backendURL); !probe.Ready {
			m.status = "Runtime is not ready. Re-open the model list and activate a local runtime."
			return m, nil, true
		}
		m.cancelTask(streamTaskKind)
		m.output = ""
		m.syncOutputViewport()
		m.status = "Streaming translation..."
		m.streaming = true
		streamReq := translate.Request{
			SourceLang:             m.sourceLang,
			TargetLang:             m.targetLang,
			Text:                   text,
			TranslationInstruction: strings.TrimSpace(m.instruction.Value()),
		}
		return m, startStreamCmd(streamReq, m.service), true
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

func (m *model) cancelAllTasks() {
	m.cancelTask(streamTaskKind)
	m.cancelTask(provisionTaskKind)
}
