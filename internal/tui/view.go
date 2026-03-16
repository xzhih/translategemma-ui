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
	colorSurface     = lipgloss.Color("236")
	colorSurfaceAlt  = lipgloss.Color("238")

	asciiBorder = lipgloss.Border{
		Top: "-", Bottom: "-", Left: "|", Right: "|",
		TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
	}

	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorInfo)
	sectionStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorSection)
	mutedStyle        = lipgloss.NewStyle().Foreground(colorMuted)
	successStyle      = lipgloss.NewStyle().Foreground(colorSuccess)
	errorStyle        = lipgloss.NewStyle().Foreground(colorError)
	bannerStyle       = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorAccent).Padding(0, 1)
	panelStyle        = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorMutedBorder).Padding(1, 1)
	panelFocusStyle   = panelStyle.Copy().BorderForeground(colorAccent)
	panelMutedStyle   = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorSurfaceAlt).Padding(1, 1)
	cardStyle         = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorSurfaceAlt).Padding(0, 1)
	fieldStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(colorSurface).Padding(0, 1)
	fieldFocusStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorAccent).Padding(0, 1)
	valueStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	badgeNeutralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(colorSurfaceAlt).Padding(0, 1)
	badgeAccentStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorAccent).Padding(0, 1)
	badgeSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorSuccess).Padding(0, 1)
	badgeWarningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorError).Padding(0, 1)
	badgeInfoStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorInfo).Padding(0, 1)
	statusPanelStyle  = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorSurfaceAlt).Padding(0, 1)
	statusActiveStyle = statusPanelStyle.Copy().BorderForeground(colorAccent)
	pathStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

func (m model) View() string {
	switch m.screen {
	case modelScreen:
		return m.renderModelScreen()
	case provisionScreen:
		return m.renderProvisionScreen()
	case translateScreen:
		return m.renderTranslateScreen()
	}
	return errorStyle.Render("invalid state")
}

func (m model) renderModelScreen() string {
	leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, _ := m.modelLayoutMetrics()
	listPanel := panelStyle.Width(leftInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		sectionStyle.Render("Runtime Catalog"),
		mutedStyle.Render("Browse local llamafile runtimes and pick what to install or activate."),
		"",
		m.modelList.View(),
	))

	detailPanel := panelStyle.Width(rightInnerWidth).Render(m.renderRuntimeDetail(rightInnerWidth))
	content := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(listPanel),
		"  ",
		lipgloss.NewStyle().Width(rightWidth).Render(detailPanel),
	)
	if m.windowWidth < 108 {
		content = lipgloss.JoinVertical(lipgloss.Left, listPanel, "", detailPanel)
	}

	return strings.Join([]string{
		m.renderBanner(),
		"",
		content,
		"",
		m.renderStatusPanel(max(28, max(leftInnerWidth, rightInnerWidth))),
		"",
		m.renderHelp(),
	}, "\n")
}

func (m model) renderProvisionScreen() string {
	panelWidth, panelInnerWidth, _ := m.provisionLayoutMetrics()
	content := panelStyle.Width(panelInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Center, m.activity.View(), " ", titleStyle.Render("Preparing local runtime")),
		mutedStyle.Render(m.currentRuntimeName()),
		"",
		m.renderProvisionStage("Download artifact", m.downloadPercent, m.downloadBar.View(), m.provisionStage == "download", "Already cached locally"),
		"",
		m.renderProvisionStage("Load runtime backend", m.loadPercent, m.loadBar.View(), m.provisionStage == "load", "Waiting for activation"),
		"",
		m.renderStatusPanel(panelInnerWidth-4),
	))

	return strings.Join([]string{
		m.renderBanner(),
		"",
		lipgloss.NewStyle().Width(panelWidth).Render(content),
		"",
		m.renderHelp(),
	}, "\n")
}

func (m model) renderTranslateScreen() string {
	summary := m.renderWorkspaceSummary()
	columns := m.renderTranslateColumns()
	if m.windowWidth < 104 {
		columns = lipgloss.JoinVertical(lipgloss.Left, columns)
	}

	return strings.Join([]string{
		m.renderBanner(),
		"",
		titleStyle.Render("Translation Workspace"),
		mutedStyle.Render(m.runtimeDescriptor()),
		"",
		summary,
		"",
		m.renderStatusPanel(clamp(m.windowWidth-8, 28, m.windowWidth)),
		"",
		sectionStyle.Render("Languages"),
		m.renderLanguageControls(),
		"",
		columns,
		"",
		m.renderHelp(),
	}, "\n")
}

func (m *model) applyLayout() {
	_, _, leftInnerWidth, rightInnerWidth, inputHeight, instructionHeight, outputHeight := m.translateLayoutMetrics()
	_, _, modelListInnerWidth, _, listHeight := m.modelLayoutMetrics()
	_, _, progressWidth := m.provisionLayoutMetrics()

	m.input.SetWidth(leftInnerWidth)
	m.input.SetHeight(inputHeight)
	m.instruction.SetWidth(leftInnerWidth)
	m.instruction.SetHeight(instructionHeight)
	m.outputView.Width = rightInnerWidth
	m.outputView.Height = outputHeight
	m.modelList.SetSize(modelListInnerWidth, listHeight)
	m.downloadBar.Width = progressWidth
	m.loadBar.Width = progressWidth
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
		lines = append(lines, mutedStyle.Render(m.runtimeDescriptor()))
	}

	width := clamp(m.windowWidth-6, 44, max(44, m.windowWidth-6))
	return bannerStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderStatus() string {
	if strings.TrimSpace(m.status) == "" {
		return mutedStyle.Render("Waiting for the next action.")
	}

	label := "Status"
	if m.shouldAnimateActivity() {
		label = m.activity.View() + " Status"
	}

	style := successStyle
	lower := strings.ToLower(m.status)
	switch {
	case strings.Contains(lower, "error"), strings.Contains(lower, "failed"), strings.Contains(lower, "unreachable"), strings.Contains(lower, "empty"), strings.Contains(lower, "refused"):
		style = errorStyle
	case strings.Contains(lower, "loading"), strings.Contains(lower, "starting"), strings.Contains(lower, "downloading"), strings.Contains(lower, "streaming"), strings.Contains(lower, "preparing"):
		style = titleStyle
	}
	return style.Render(label + ": " + m.status)
}

func (m model) renderStatusPanel(width int) string {
	if width < 24 {
		width = 24
	}
	style := statusPanelStyle
	if m.shouldAnimateActivity() {
		style = statusActiveStyle
	}
	return style.Width(width).Render(m.renderStatus())
}

func (m model) renderHelp() string {
	helpView := m.help
	helpView.Width = clamp(m.windowWidth-8, 20, m.windowWidth)
	return mutedStyle.Render(helpView.ShortHelpView(m.shortHelpBindings()))
}

func (m model) renderRuntimeDetail(width int) string {
	item, ok := m.currentCatalogItem()
	if !ok {
		return mutedStyle.Render("No runtime metadata available.")
	}

	lines := []string{
		sectionStyle.Render("Selected Runtime"),
		valueStyle.Render(item.FileName),
		mutedStyle.Render("Quantized local runtime package"),
		"",
		renderBadgeRow(runtimeBadges(item, item.Active && m.runtimeReady)),
		"",
		m.renderKeyValue("Size", item.Size),
		m.renderKeyValue("Availability", runtimeAvailability(item)),
		m.renderKeyValue("Startup flow", runtimeStartupHint(item)),
	}

	if item.Path != "" {
		lines = append(lines,
			"",
			mutedStyle.Render("Local path"),
			pathStyle.Width(width).Render(item.Path),
		)
	}

	action := "Press Enter to download and activate this runtime."
	if item.Installed {
		action = "Press Enter to activate this local runtime."
	}
	lines = append(lines, "", mutedStyle.Render(action))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m model) renderKeyValue(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		mutedStyle.Width(14).Render(label),
		valueStyle.Render(value),
	)
}

func (m model) renderProvisionStage(label string, percent float64, bar string, active bool, fallback string) string {
	style := panelMutedStyle
	if active {
		style = panelFocusStyle
	}

	lines := []string{sectionStyle.Render(label)}
	switch {
	case percent < 0:
		lines = append(lines, mutedStyle.Render(fallback))
	default:
		lines = append(lines, fmt.Sprintf("%s  %3.0f%%", bar, percent))
		if active {
			lines = append(lines, mutedStyle.Render("Current step is active."))
		}
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m model) renderWorkspaceSummary() string {
	cards := []string{
		m.renderSummaryCard("Runtime", m.currentRuntimeName(), m.runtimeStateLabel()),
		m.renderSummaryCard("Direction", fmt.Sprintf("%s -> %s", strings.ToUpper(m.sourceLang), strings.ToUpper(m.targetLang)), fmt.Sprintf("%s -> %s", languages.Label(m.sourceLang), languages.Label(m.targetLang))),
		m.renderSummaryCard("Activity", m.activityStateLabel(), m.activityCaption()),
	}

	if m.windowWidth < 100 {
		return lipgloss.JoinVertical(lipgloss.Left, cards...)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards[0], "  ", cards[1], "  ", cards[2])
}

func (m model) renderSummaryCard(label, value, caption string) string {
	width := clamp((m.windowWidth-12)/3, 22, 38)
	return cardStyle.Width(width).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		sectionStyle.Render(label),
		valueStyle.Render(value),
		mutedStyle.Render(caption),
	))
}

func (m model) renderLanguageControls() string {
	source := m.renderLanguageField("Source", m.sourceLang, m.focus == sourceFocus)
	target := m.renderLanguageField("Target", m.targetLang, m.focus == targetFocus)
	arrow := mutedStyle.Render("  ->  ")
	if m.windowWidth < 84 {
		return source + "\n" + target
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, source, arrow, target)
}

func (m model) renderLanguageField(label, code string, focused bool) string {
	content := fmt.Sprintf("%s: %s (%s)", label, languages.Label(code), code)
	if focused {
		return fieldFocusStyle.Render(content)
	}
	return fieldStyle.Render(content)
}

func (m model) renderTranslateColumns() string {
	leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, _, _, _ := m.translateLayoutMetrics()

	inputPanel := panelStyle.Width(leftInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		sectionStyle.Render("Source Text"),
		m.input.View(),
		"",
		sectionStyle.Render("Instruction"),
		m.instruction.View(),
	))

	outputPanelStyle := panelStyle
	if m.focus == outputFocus {
		outputPanelStyle = panelFocusStyle
	}
	outputPanel := outputPanelStyle.Width(rightInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		sectionStyle.Render("Translation Output"),
		m.outputView.View(),
	))

	if m.windowWidth < 104 {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().Width(leftWidth).Render(inputPanel),
			"",
			lipgloss.NewStyle().Width(rightWidth).Render(outputPanel),
		)
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(inputPanel),
		"  ",
		lipgloss.NewStyle().Width(rightWidth).Render(outputPanel),
	)
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

func (m model) runtimeDescriptor() string {
	name := m.currentRuntimeName()
	if name == "" || name == "none" {
		return "Selected runtime: none"
	}
	if m.runtimeReady {
		return "Loaded runtime: " + name
	}
	return "Selected runtime: " + name
}

func (m model) runtimeStateLabel() string {
	if m.runtimeReady {
		return "Loaded and ready"
	}
	if strings.TrimSpace(m.currentRuntimeName()) == "" || m.currentRuntimeName() == "none" {
		return "No runtime selected"
	}
	return "Selected, lazy loaded"
}

func (m model) activityStateLabel() string {
	switch {
	case m.screen == provisionScreen:
		return "Provisioning"
	case m.streaming:
		return "Streaming"
	case m.runtimeReady:
		return "Idle"
	default:
		return "Standby"
	}
}

func (m model) activityCaption() string {
	switch {
	case m.screen == provisionScreen:
		return "Preparing the local backend"
	case m.streaming:
		return "Translation is in progress"
	case m.runtimeReady:
		return "Backend is warm and ready"
	default:
		return "The first request will auto-load"
	}
}

func runtimeBadges(item modelstore.CatalogItem, loaded bool) []string {
	badges := make([]string, 0, 4)
	if item.Recommended {
		badges = append(badges, "RECOMMENDED")
	}
	if item.Installed {
		badges = append(badges, "LOCAL")
	} else {
		badges = append(badges, "DOWNLOAD")
	}
	if item.Active {
		badges = append(badges, "SELECTED")
	}
	if loaded {
		badges = append(badges, "LOADED")
	}
	return badges
}

func runtimeAvailability(item modelstore.CatalogItem) string {
	if item.Installed {
		return "Cached locally"
	}
	return "Needs download"
}

func runtimeStartupHint(item modelstore.CatalogItem) string {
	if item.Installed {
		return "Activates immediately"
	}
	return "Downloads, then activates"
}

func renderBadgeRow(labels []string) string {
	rendered := make([]string, 0, len(labels))
	for _, label := range labels {
		rendered = append(rendered, renderBadge(label))
	}
	return strings.Join(rendered, " ")
}

func renderBadge(label string) string {
	switch label {
	case "RECOMMENDED":
		return badgeAccentStyle.Render(label)
	case "LOCAL", "LOADED":
		return badgeSuccessStyle.Render(label)
	case "SELECTED":
		return badgeInfoStyle.Render(label)
	case "DOWNLOAD":
		return badgeWarningStyle.Render(label)
	default:
		return badgeNeutralStyle.Render(label)
	}
}

func (m model) modelLayoutMetrics() (leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, listHeight int) {
	contentWidth := clamp(m.windowWidth-8, 48, max(48, m.windowWidth-8))
	gap := 2
	leftWidth = int(float64(contentWidth) * 0.58)
	if leftWidth < 28 {
		leftWidth = 28
	}
	rightWidth = contentWidth - gap - leftWidth
	if rightWidth < 24 {
		rightWidth = 24
		leftWidth = contentWidth - gap - rightWidth
	}
	leftInnerWidth = max(24, leftWidth-4)
	rightInnerWidth = max(20, rightWidth-4)
	listHeight = clamp(m.windowHeight-18, 12, max(12, m.windowHeight-18))
	return leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, listHeight
}

func (m model) provisionLayoutMetrics() (panelWidth, panelInnerWidth, progressWidth int) {
	panelWidth = clamp(m.windowWidth-10, 42, 86)
	panelInnerWidth = max(34, panelWidth-4)
	progressWidth = clamp(panelInnerWidth-8, 18, 46)
	return panelWidth, panelInnerWidth, progressWidth
}

func (m model) translateLayoutMetrics() (leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, inputHeight, instructionHeight, outputHeight int) {
	contentWidth := clamp(m.windowWidth-8, 42, max(42, m.windowWidth-8))
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

	leftInnerWidth = max(14, leftWidth-4)
	rightInnerWidth = max(14, rightWidth-4)

	columnHeight := clamp(m.windowHeight-23, 14, max(14, m.windowHeight-23))
	instructionHeight = clamp(columnHeight/4, 4, 7)
	inputHeight = max(7, columnHeight-instructionHeight-6)
	outputHeight = max(10, columnHeight+1)
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
