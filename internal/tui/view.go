package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/modelstore"
)

var (
	colorAccent      = lipgloss.Color("45")
	colorInfo        = lipgloss.Color("117")
	colorSection     = lipgloss.Color("221")
	colorMuted       = lipgloss.Color("244")
	colorMutedBorder = lipgloss.Color("239")
	colorSuccess     = lipgloss.Color("42")
	colorWarning     = lipgloss.Color("214")
	colorError       = lipgloss.Color("203")
	colorSurface     = lipgloss.Color("236")
	colorSurfaceAlt  = lipgloss.Color("238")

	asciiBorder = lipgloss.Border{
		Top: "-", Bottom: "-", Left: "|", Right: "|",
		TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
	}

	heroStyle         = lipgloss.NewStyle().Bold(true).Foreground(colorInfo)
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorSection)
	mutedStyle        = lipgloss.NewStyle().Foreground(colorMuted)
	successStyle      = lipgloss.NewStyle().Foreground(colorSuccess)
	errorStyle        = lipgloss.NewStyle().Foreground(colorError)
	panelStyle        = lipgloss.NewStyle().BorderStyle(asciiBorder).BorderForeground(colorMutedBorder).Padding(0, 1)
	panelFocusStyle   = panelStyle.Copy().BorderForeground(colorAccent)
	fieldStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(colorSurface).Padding(0, 1)
	fieldFocusStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorAccent).Padding(0, 1)
	valueStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	pathStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	badgeNeutralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(colorSurfaceAlt).Padding(0, 1)
	badgeAccentStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorAccent).Padding(0, 1)
	badgeSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorSuccess).Padding(0, 1)
	badgeWarningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorWarning).Padding(0, 1)
	badgeErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorError).Padding(0, 1)
	badgeInfoStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(colorInfo).Padding(0, 1)
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
		m.renderPanelHeader("Runtime Library", fmt.Sprintf("%d local / %d total", m.installedRuntimeCount(), len(m.catalog)), renderPill("ENTER", "neutral")),
		m.modelList.View(),
	))

	detailPanel := panelStyle.Width(rightInnerWidth).Render(m.renderRuntimeDetail(rightInnerWidth))
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(listPanel),
		"  ",
		lipgloss.NewStyle().Width(rightWidth).Render(detailPanel),
	)
	if m.windowWidth < 78 {
		content = lipgloss.JoinVertical(lipgloss.Left, listPanel, detailPanel)
	}

	return m.renderShell(
		"Runtime Library",
		m.runtimeDescriptor(),
		content,
	)
}

func (m model) renderProvisionScreen() string {
	panelWidth, panelInnerWidth, _ := m.provisionLayoutMetrics()
	content := panelStyle.Width(panelInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderPanelHeader("Warming Runtime", "", renderPill("AUTO", "neutral")),
		m.renderInfoLine("Model", m.currentRuntimeName()),
		m.renderInfoLine("Status", m.status),
		"",
		m.renderProvisionStage("Download", m.downloadPercent, m.provisionStage == "download", "cached", m.renderDownloadMetrics()),
		"",
		m.renderProvisionStage("Load", m.loadPercent, m.provisionStage == "load", "", ""),
	))

	return m.renderProvisionShell(lipgloss.NewStyle().Width(panelWidth).Render(content))
}

func (m model) renderTranslateScreen() string {
	columns := m.renderTranslateColumns()

	return m.renderTranslateShell(lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderLanguageToolbar(),
		columns,
	))
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
		content = m.renderOutputPlaceholder(width)
	}
	m.outputView.SetContent(lipgloss.NewStyle().Width(width).Render(content))
	if m.outputView.TotalLineCount() <= m.outputView.Height {
		m.outputView.GotoTop()
		return
	}
	m.outputView.GotoBottom()
}

func (m model) renderShell(screenTitle, context, body string) string {
	lines := []string{m.renderHeader(screenTitle, context), body, m.renderStatusPanel(clamp(m.windowWidth-2, 24, m.windowWidth))}
	if m.showHelpLine() {
		lines = append(lines, m.renderHelp())
	}
	return strings.Join(lines, "\n")
}

func (m model) renderTranslateShell(body string) string {
	width := clamp(m.windowWidth-2, 24, m.windowWidth)
	bodyHeight := max(1, m.windowHeight-m.translateChromeHeight())
	lines := []string{
		m.renderAppBanner(),
		lipgloss.NewStyle().Width(width).Height(bodyHeight).Render(body),
	}
	if m.showHelpLine() {
		lines = append(lines, m.renderHelp())
	}
	lines = append(lines, m.renderFooterContext())
	return strings.Join(lines, "\n")
}

func (m model) renderProvisionShell(body string) string {
	width := clamp(m.windowWidth-2, 24, m.windowWidth)
	bodyHeight := max(1, m.windowHeight-m.translateChromeHeight())
	lines := []string{
		m.renderAppBanner(),
		lipgloss.Place(width, bodyHeight, lipgloss.Center, lipgloss.Top, body),
	}
	if m.showHelpLine() {
		lines = append(lines, m.renderHelp())
	}
	lines = append(lines, m.renderFooterContext())
	return strings.Join(lines, "\n")
}

func (m model) renderAppBanner() string {
	width := clamp(m.windowWidth-2, 24, m.windowWidth)
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(heroStyle.Render("TranslateGemmaUI"))
}

func (m model) renderHeader(screenTitle, context string) string {
	parts := []string{heroStyle.Render("TranslateGemmaUI")}
	if strings.TrimSpace(screenTitle) != "" {
		parts = append(parts, mutedStyle.Render(screenTitle))
	}
	if strings.TrimSpace(context) != "" {
		parts = append(parts, mutedStyle.Render(context))
	}
	line := strings.Join(parts, "  |  ")
	return ansi.Truncate(line, clamp(m.windowWidth-2, 24, m.windowWidth-2), "…")
}

func (m model) renderFooterContext() string {
	width := clamp(m.windowWidth-2, 24, m.windowWidth)
	left, right := m.renderFooterMeta()
	leftWidth := max(1, width-lipgloss.Width(right)-1)
	left = ansi.Truncate(left, leftWidth, "…")
	gapWidth := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gapWidth) + right
}

func (m model) renderStatus() string {
	message := strings.TrimSpace(m.status)
	if message == "" {
		message = "Waiting for the next action."
	}
	tone := statusTone(message, m.shouldAnimateActivity())
	label := "Status"
	switch tone {
	case "success":
		label = "Ready"
	case "error":
		label = "Issue"
	case "warning":
		label = "Attention"
	case "accent":
		label = "Working"
	}
	return renderPill(strings.ToUpper(label), tone) + " " + message
}

func (m model) renderStatusPanel(width int) string {
	return ansi.Truncate(m.renderStatus(), width, "…")
}

func (m model) renderHelp() string {
	helpView := m.help
	helpView.Width = clamp(m.windowWidth-8, 20, m.windowWidth)
	line := mutedStyle.Render("Keys ") + helpView.ShortHelpView(m.shortHelpBindings())
	return ansi.Truncate(line, clamp(m.windowWidth-2, 20, m.windowWidth-2), "...")
}

func (m model) renderRuntimeDetail(width int) string {
	item, ok := m.currentCatalogItem()
	if !ok {
		return mutedStyle.Render("No runtime metadata available.")
	}

	lines := []string{
		m.renderPanelHeader("Selection", item.ID, renderPill(runtimeCapability(item.QuantizedModel), capabilityTone(item.QuantizedModel))),
		valueStyle.Render(item.FileName),
		renderBadgeRow(runtimeBadges(item, item.Active && m.runtimeReady)),
		"",
		m.renderInfoLine("Status", runtimeSelectionStatus(item, item.Active && m.runtimeReady)),
	}

	if item.Path != "" {
		lines = append(lines, "",
			m.renderInfoLine("Path", filepath.Base(item.Path)),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m model) renderInfoLine(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		mutedStyle.Width(12).Render(label),
		valueStyle.Render(value),
	)
}

func (m model) renderProvisionStage(label string, percent float64, active bool, fallback, detail string) string {
	stateLabel, tone := provisionState(percent, active)
	style := panelStyle
	if active {
		style = panelFocusStyle
	} else if percent >= 100 {
		style = panelStyle.Copy().BorderForeground(colorSuccess)
	}

	bar := m.loadBar.View()
	if strings.Contains(strings.ToLower(label), "download") {
		bar = m.downloadBar.View()
	}

	lines := []string{
		m.renderPanelHeader(label, "", renderPill(stateLabel, tone)),
	}
	switch {
	case percent < 0:
		if strings.TrimSpace(fallback) != "" {
			lines = append(lines, mutedStyle.Render(fallback))
		}
	default:
		lines = append(lines, fmt.Sprintf("%s  %3.0f%%", bar, percent))
		if strings.TrimSpace(detail) != "" {
			lines = append(lines, mutedStyle.Render(detail))
		}
		if !active && percent >= 100 {
			lines = append(lines, successStyle.Render("Done"))
		}
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m model) renderLanguageToolbar() string {
	left := m.renderLanguageField(m.sourceLang, m.focus == sourceFocus)
	right := m.renderLanguageField(m.targetLang, m.focus == targetFocus)
	line := lipgloss.JoinHorizontal(
		lipgloss.Center,
		left,
		" ",
		mutedStyle.Render("->"),
		" ",
		right,
	)
	return ansi.Truncate(line, clamp(m.windowWidth-2, 24, m.windowWidth-2), "…")
}

func (m model) renderLanguageField(code string, focused bool) string {
	content := fmt.Sprintf("%s (%s)", languages.Label(code), code)
	if focused {
		return fieldFocusStyle.Render(content)
	}
	return fieldStyle.Render(content)
}

func (m model) renderTranslateColumns() string {
	leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, _, _, _ := m.translateLayoutMetrics()

	sourcePanelStyle := panelStyle
	if m.focus == textFocus {
		sourcePanelStyle = panelFocusStyle
	}
	sourcePanel := sourcePanelStyle.Width(leftInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderPanelHeader("Source Text", textMetrics(strings.TrimSpace(m.input.Value())), focusBadge(m.focus == textFocus)),
		m.input.View(),
	))

	instructionPanelStyle := panelStyle
	if m.focus == instructionFocus {
		instructionPanelStyle = panelFocusStyle
	}
	instructionPanel := instructionPanelStyle.Width(leftInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderPanelHeader("Instruction", textMetrics(strings.TrimSpace(m.instruction.Value())), focusBadge(m.focus == instructionFocus)),
		m.instruction.View(),
	))

	leftColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		sourcePanel,
		instructionPanel,
	)

	outputPanelStyle := panelStyle
	if m.focus == outputFocus {
		outputPanelStyle = panelFocusStyle
	}
	outputPanel := outputPanelStyle.Width(rightInnerWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderPanelHeader("Translation Output", textMetrics(strings.TrimSpace(m.output)), renderPill(strings.ToUpper(m.outputStateLabel()), outputStateTone(m))),
		m.outputView.View(),
	))

	if m.windowWidth < 68 {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().Width(leftWidth).Render(leftColumn),
			lipgloss.NewStyle().Width(rightWidth).Render(outputPanel),
		)
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(leftColumn),
		"  ",
		lipgloss.NewStyle().Width(rightWidth).Render(outputPanel),
	)
}

func (m model) renderOutputPlaceholder(width int) string {
	lines := []string{}
	if m.streaming {
		lines = append(lines, heroStyle.Render("Streaming translation..."))
	} else {
		lines = append(lines, mutedStyle.Render("Press Ctrl+R to translate."))
	}
	if !m.runtimeReady {
		lines = append(lines, mutedStyle.Render("Runtime warms on first run."))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderPanelHeader(title, meta, badge string) string {
	left := titleStyle.Render(title)
	if meta != "" {
		left += "  " + mutedStyle.Render(meta)
	}
	if badge == "" {
		return left
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, left, "  ", badge)
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

func (m model) renderFooterMeta() (string, string) {
	modelName := strings.TrimSpace(m.currentRuntimeName())
	if modelName == "" {
		modelName = "none"
	}
	left := lipgloss.JoinHorizontal(
		lipgloss.Left,
		mutedStyle.Render("Model "),
		valueStyle.Render(modelName),
	)
	right := lipgloss.JoinHorizontal(
		lipgloss.Left,
		mutedStyle.Render("Status "),
		m.footerStatusStyle().Render(m.footerStatusLabel()),
	)
	return left, right
}

func (m model) footerStatusLabel() string {
	switch {
	case m.screen == provisionScreen:
		return "warming"
	case m.runtimeReady:
		return "ready"
	default:
		return "idle"
	}
}

func (m model) footerStatusStyle() lipgloss.Style {
	switch m.footerStatusLabel() {
	case "ready":
		return successStyle
	case "warming":
		return heroStyle
	case "idle":
		return lipgloss.NewStyle().Foreground(colorWarning)
	default:
		return valueStyle
	}
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

func (m model) outputStateLabel() string {
	switch {
	case statusTone(m.status, false) == "error":
		return "Error"
	case m.streaming:
		return "Streaming"
	case strings.TrimSpace(m.output) != "":
		return "Done"
	default:
		return "Empty"
	}
}

func runtimeBadges(item modelstore.CatalogItem, loaded bool) []string {
	badges := make([]string, 0, 5)
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
	if models.SupportsVision(item.QuantizedModel) {
		badges = append(badges, "VISION")
	}
	return badges
}

func runtimeAvailability(item modelstore.CatalogItem) string {
	if item.Installed {
		return "Cached locally"
	}
	return "Not downloaded yet"
}

func runtimeStartupHint(item modelstore.CatalogItem) string {
	if item.Installed {
		return "Warm on first translation"
	}
	return "Download first, then warm automatically"
}

func runtimeCapability(item models.QuantizedModel) string {
	if models.SupportsVision(item) {
		return "TEXT + IMAGE"
	}
	return "TEXT ONLY"
}

func capabilityTone(item models.QuantizedModel) string {
	if models.SupportsVision(item) {
		return "info"
	}
	return "success"
}

func runtimeUseCase(item models.QuantizedModel) string {
	if models.SupportsVision(item) {
		return "image translation and text tasks"
	}
	switch strings.ToLower(strings.TrimSpace(item.ID)) {
	case "q4_k_m":
		return "balanced day-to-day translation"
	case "q6_k":
		return "higher quality text translation"
	case "q8_0":
		return "best text quality in the bundled set"
	default:
		return "general local translation workloads"
	}
}

func runtimeActionPrompt(item modelstore.CatalogItem, loaded bool) string {
	switch {
	case loaded:
		return "Already loaded"
	case item.Installed:
		return "Select for next translation"
	default:
		return "Download and select"
	}
}

func runtimeSelectionStatus(item modelstore.CatalogItem, loaded bool) string {
	switch {
	case loaded:
		return "Loaded"
	case item.Installed:
		return "Local"
	default:
		return "Download required"
	}
}

func compactRuntimeName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "none" {
		return name
	}
	name = strings.TrimSuffix(name, ".llamafile")
	name = strings.TrimPrefix(name, "translategemma-4b-it.")
	return name
}

func (m model) renderDownloadMetrics() string {
	if m.downloadedBytes <= 0 && m.downloadTotal <= 0 && m.downloadSpeed <= 0 {
		return ""
	}
	return fmt.Sprintf(
		"%s / %s  %s",
		humanBytes(m.downloadedBytes),
		humanBytes(m.downloadTotal),
		humanSpeed(m.downloadSpeed),
	)
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
	case "SELECTED", "VISION":
		return badgeInfoStyle.Render(label)
	case "DOWNLOAD":
		return badgeWarningStyle.Render(label)
	default:
		return badgeNeutralStyle.Render(label)
	}
}

func renderPill(label, tone string) string {
	switch tone {
	case "accent":
		return badgeAccentStyle.Render(label)
	case "success":
		return badgeSuccessStyle.Render(label)
	case "warning":
		return badgeWarningStyle.Render(label)
	case "error":
		return badgeErrorStyle.Render(label)
	case "info":
		return badgeInfoStyle.Render(label)
	default:
		return badgeNeutralStyle.Render(label)
	}
}

func focusBadge(focused bool) string {
	_ = focused
	return ""
}

func textMetrics(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "empty"
	}
	return fmt.Sprintf("%d chars | %d words", utf8.RuneCountInString(text), len(strings.Fields(text)))
}

func statusTone(message string, active bool) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "error"), strings.Contains(lower, "failed"), strings.Contains(lower, "unreachable"), strings.Contains(lower, "refused"):
		return "error"
	case active, strings.Contains(lower, "loading"), strings.Contains(lower, "starting"), strings.Contains(lower, "downloading"), strings.Contains(lower, "streaming"), strings.Contains(lower, "preparing"):
		return "accent"
	case strings.Contains(lower, "idle"), strings.Contains(lower, "empty"):
		return "warning"
	default:
		return "success"
	}
}

func provisionState(percent float64, active bool) (string, string) {
	switch {
	case active:
		return "ACTIVE", "accent"
	case percent >= 100:
		return "DONE", "success"
	case percent >= 0:
		return "QUEUED", "info"
	default:
		return "SKIPPED", "neutral"
	}
}

func outputStateTone(m model) string {
	switch {
	case statusTone(m.status, false) == "error":
		return "error"
	case m.streaming:
		return "accent"
	case strings.TrimSpace(m.output) != "":
		return "success"
	default:
		return "neutral"
	}
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "--"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
		tb = 1024 * gb
	)
	switch {
	case n >= tb:
		return fmt.Sprintf("%.2f TB", float64(n)/float64(tb))
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func humanSpeed(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return "--/s"
	}
	return fmt.Sprintf("%s/s", humanBytes(int64(bytesPerSecond)))
}

func (m model) installedRuntimeCount() int {
	count := 0
	for _, item := range m.catalog {
		if item.Installed {
			count++
		}
	}
	return count
}

func (m model) modelLayoutMetrics() (leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, listHeight int) {
	contentWidth := clamp(m.windowWidth-2, 48, max(48, m.windowWidth-2))
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
	listHeight = clamp(m.windowHeight-m.shellChromeHeight()-3, 8, max(8, m.windowHeight-m.shellChromeHeight()-3))
	return leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, listHeight
}

func (m model) provisionLayoutMetrics() (panelWidth, panelInnerWidth, progressWidth int) {
	panelWidth = clamp(m.windowWidth-4, 40, 86)
	panelInnerWidth = max(32, panelWidth-4)
	progressWidth = clamp(panelInnerWidth-8, 18, 46)
	return panelWidth, panelInnerWidth, progressWidth
}

func (m model) translateLayoutMetrics() (leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, inputHeight, instructionHeight, outputHeight int) {
	contentWidth := clamp(m.windowWidth-2, 42, max(42, m.windowWidth-2))
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

	columnHeight := clamp(m.windowHeight-m.translateChromeHeight()-2, 14, max(14, m.windowHeight-m.translateChromeHeight()-2))
	instructionHeight = 3
	if columnHeight >= 24 {
		instructionHeight = 4
	}
	if columnHeight >= 30 {
		instructionHeight = 5
	}
	inputHeight = clamp(columnHeight-instructionHeight-7, 5, 18)
	outputHeight = inputHeight + instructionHeight + 2
	return leftWidth, rightWidth, leftInnerWidth, rightInnerWidth, inputHeight, instructionHeight, outputHeight
}

func (m model) translateChromeHeight() int {
	height := 2
	if m.showHelpLine() {
		height++
	}
	return height
}

func (m model) shellChromeHeight() int {
	height := 2
	if m.showHelpLine() {
		height++
	}
	return height
}

func (m model) showHelpLine() bool {
	return m.windowHeight >= 28 && m.windowWidth >= 96
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
		m.status = "Reviewing translation output"
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

func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
