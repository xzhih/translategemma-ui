package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/modelstore"
)

var (
	colorAccent      = lipgloss.Color("63")
	colorInfo        = lipgloss.Color("12")
	colorSection     = lipgloss.Color("81")
	colorMuted       = lipgloss.Color("245")
	colorMutedBorder = lipgloss.Color("240")
	colorSuccess     = lipgloss.Color("42")
	colorError       = lipgloss.Color("160")

	asciiBorder     = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorInfo)
	sectionStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorSection)
	mutedStyle      = lipgloss.NewStyle().Foreground(colorMuted)
	successStyle    = lipgloss.NewStyle().Foreground(colorSuccess)
	errorStyle      = lipgloss.NewStyle().Foreground(colorError)
	bannerStyle     = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorAccent).Padding(0, 1)
	panelStyle      = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorMutedBorder).Padding(0, 1)
	fieldStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Padding(0, 1)
	fieldFocusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorAccent).Padding(0, 1)
)

func (m model) View() string {
	switch m.screen {
	case modelScreen:
		var b strings.Builder
		b.WriteString(m.renderBanner())
		b.WriteString("\n\n")
		b.WriteString(titleStyle.Render("Select a quantized model"))
		b.WriteString("\n\n")
		catalog := modelstore.Catalog(m.dataRoot, m.models, strings.TrimSpace(m.cfg.ActiveModelID), strings.TrimSpace(m.state.ActiveModelPath))
		for i, item := range catalog {
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			status := "download"
			if item.Installed {
				status = "installed"
			}
			if item.Active {
				status += " | active"
			}
			b.WriteString(fmt.Sprintf("%s %s (%s | %s)\n", cursor, item.FileName, item.Size, status))
		}
		b.WriteString("\n")
		b.WriteString(m.renderStatus())
		b.WriteString("\n\n")
		b.WriteString(m.renderHelp())
		return b.String()

	case provisionScreen:
		var b strings.Builder
		b.WriteString(m.renderBanner())
		b.WriteString("\n\n")
		b.WriteString(titleStyle.Render("Preparing local runtime"))
		b.WriteString("\n\n")
		b.WriteString(m.renderProgressLine("Download", m.downloadPercent, "Already installed locally"))
		b.WriteString("\n")
		b.WriteString(m.renderProgressLine("Load", m.loadPercent, "Waiting to start"))
		b.WriteString("\n\n")
		if m.provisionStage != "" {
			b.WriteString(sectionStyle.Render("Current stage: " + m.provisionStage))
			b.WriteString("\n")
		}
		b.WriteString(m.renderStatus())
		b.WriteString("\n\n")
		b.WriteString(m.renderHelp())
		return b.String()

	case translateScreen:
		var b strings.Builder
		b.WriteString(m.renderBanner())
		b.WriteString("\n\n")
		b.WriteString(titleStyle.Render("Translation Workspace"))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Runtime: %s", m.currentRuntimeName())))
		b.WriteString("\n")
		b.WriteString(m.renderStatus())
		b.WriteString("\n\n")
		b.WriteString(sectionStyle.Render("Languages"))
		b.WriteString("\n")
		b.WriteString(m.renderLanguageControls())
		b.WriteString("\n\n")
		b.WriteString(m.renderTranslateColumns())
		b.WriteString("\n\n")
		b.WriteString(m.renderHelp())
		return b.String()
	}

	return errorStyle.Render("invalid state")
}

func (m *model) applyLayout() {
	_, _, leftInnerWidth, rightInnerWidth, inputHeight, instructionHeight, outputHeight := m.translateLayoutMetrics()
	m.input.SetWidth(leftInnerWidth)
	m.input.SetHeight(inputHeight)
	m.instruction.SetWidth(leftInnerWidth)
	m.instruction.SetHeight(instructionHeight)
	m.outputView.Width = rightInnerWidth
	m.outputView.Height = outputHeight
	m.syncOutputViewport()
}

func (m *model) syncOutputViewport() {
	width := m.outputView.Width
	if width <= 0 {
		width = 72
	}
	content := strings.TrimRight(m.output, "\n")
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("Translation output will appear here.")
	}
	m.outputView.SetContent(lipgloss.NewStyle().Width(width).Render(content))
	if m.outputView.TotalLineCount() <= m.outputView.Height {
		m.outputView.GotoTop()
		return
	}
	m.outputView.GotoBottom()
}

func (m model) renderBanner() string {
	lines := []string{
		titleStyle.Render("TranslateGemmaUI"),
		"Local TranslateGemma runtime for fast bilingual translation workflows.",
	}
	if runtimeName := m.currentRuntimeName(); runtimeName != "" {
		lines = append(lines, mutedStyle.Render("Active runtime: "+runtimeName))
	}
	width := m.windowWidth - 6
	if width < 40 {
		width = 40
	}
	return bannerStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderStatus() string {
	if strings.TrimSpace(m.status) == "" {
		return ""
	}
	style := successStyle
	lower := strings.ToLower(m.status)
	switch {
	case strings.Contains(lower, "error"), strings.Contains(lower, "failed"), strings.Contains(lower, "unreachable"), strings.Contains(lower, "empty"):
		style = errorStyle
	case strings.Contains(lower, "loading"), strings.Contains(lower, "starting"), strings.Contains(lower, "downloading"), strings.Contains(lower, "streaming"):
		style = titleStyle
	}
	return style.Render("Status: " + m.status)
}

func (m model) renderHelp() string {
	helpView := m.help
	helpView.Width = clamp(m.windowWidth-8, 20, m.windowWidth)
	return mutedStyle.Render(helpView.ShortHelpView(m.shortHelpBindings()))
}

func (m model) renderProgressLine(label string, percent float64, fallback string) string {
	if percent < 0 {
		return mutedStyle.Render(label + ": " + fallback)
	}
	return fmt.Sprintf("%-9s %s %3.0f%%", label+":", progressBar(percent, 34), percent)
}

func progressBar(percent float64, width int) string {
	if width < 10 {
		width = 10
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := int((percent / 100) * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func (m model) currentRuntimeName() string {
	if strings.TrimSpace(m.selectedName) != "" {
		return m.selectedName
	}
	if strings.TrimSpace(m.state.ActiveModelPath) != "" {
		return filepath.Base(m.state.ActiveModelPath)
	}
	return "none"
}

func (m model) renderLanguageControls() string {
	source := m.renderLanguageField("Source", m.sourceLang, m.focus == sourceFocus)
	target := m.renderLanguageField("Target", m.targetLang, m.focus == targetFocus)
	if m.windowWidth < 84 {
		return source + "\n" + target
	}
	return source + "  " + target
}

func (m model) renderLanguageField(label, code string, focused bool) string {
	content := fmt.Sprintf("%s: %s (%s)", label, languages.Label(code), code)
	if focused {
		return fieldFocusStyle.Render(content)
	}
	return fieldStyle.Render(content)
}

func (m model) renderTranslateColumns() string {
	leftWidth, rightWidth, _, rightInnerWidth, _, _, _ := m.translateLayoutMetrics()
	left := lipgloss.NewStyle().
		Width(leftWidth).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			sectionStyle.Render("Input"),
			m.input.View(),
			"",
			sectionStyle.Render("Instruction (optional)"),
			m.instruction.View(),
		))

	outputPanel := panelStyle.Width(rightInnerWidth)
	if m.focus == outputFocus {
		outputPanel = outputPanel.BorderForeground(colorAccent)
	}
	right := lipgloss.NewStyle().
		Width(rightWidth).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			sectionStyle.Render("Output"),
			outputPanel.Render(m.outputView.View()),
		))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m model) translateLayoutMetrics() (leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, inputHeight, instructionHeight, outputHeight int) {
	contentWidth := m.windowWidth - 8
	if contentWidth < 42 {
		contentWidth = 42
	}
	gap := 2
	leftWidth = (contentWidth - gap) / 2
	rightWidth = contentWidth - gap - leftWidth
	if leftWidth < 20 {
		leftWidth = 20
		rightWidth = contentWidth - gap - leftWidth
	}
	if rightWidth < 20 {
		rightWidth = 20
		leftWidth = contentWidth - gap - rightWidth
	}
	if leftWidth < 18 {
		leftWidth = 18
	}
	if rightWidth < 18 {
		rightWidth = 18
	}

	leftInnerWidth = leftWidth - 4
	if leftInnerWidth < 14 {
		leftInnerWidth = 14
	}
	rightInnerWidth = rightWidth - 4
	if rightInnerWidth < 14 {
		rightInnerWidth = 14
	}

	columnHeight := m.windowHeight - 18
	if columnHeight < 14 {
		columnHeight = 14
	}
	instructionHeight = clamp(columnHeight/4, 4, 7)
	inputHeight = columnHeight - instructionHeight - 6
	if inputHeight < 7 {
		inputHeight = 7
	}
	outputHeight = columnHeight + 1
	if outputHeight < 10 {
		outputHeight = 10
	}
	return leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, inputHeight, instructionHeight, outputHeight
}

func (m *model) rotateFocus(delta int) tea.Cmd {
	order := []focusField{textFocus, instructionFocus, outputFocus, sourceFocus, targetFocus}
	current := 0
	for idx, item := range order {
		if item == m.focus {
			current = idx
			break
		}
	}
	next := (current + delta + len(order)) % len(order)
	return m.setFocus(order[next])
}

func (m *model) setFocus(next focusField) tea.Cmd {
	m.focus = next
	m.input.Blur()
	m.instruction.Blur()
	switch next {
	case textFocus:
		m.status = "Editing source text"
		return m.input.Focus()
	case instructionFocus:
		m.status = "Editing translation instruction"
		return m.instruction.Focus()
	case outputFocus:
		m.status = "Scrolling translation output"
	case sourceFocus:
		m.status = "Selecting source language"
	case targetFocus:
		m.status = "Selecting target language"
	}
	return nil
}

func (m *model) changeFocusedLanguage(delta int) {
	switch m.focus {
	case sourceFocus:
		m.sourceLang = cycleLanguage(m.sourceLang, m.sourceOptions, delta)
		m.status = fmt.Sprintf("Source language: %s", languages.Label(m.sourceLang))
	case targetFocus:
		m.targetLang = cycleLanguage(m.targetLang, m.targetOptions, delta)
		m.status = fmt.Sprintf("Target language: %s", languages.Label(m.targetLang))
	}
}

func cycleLanguage(current string, options []languages.Option, delta int) string {
	if len(options) == 0 {
		return current
	}
	idx := 0
	for i, item := range options {
		if item.Code == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(options)) % len(options)
	return options[idx].Code
}

func clamp(v, minValue, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}
