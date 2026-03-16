package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/huggingface"
	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/modelstore"
	"translategemma-ui/internal/runtime"
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/translate"
)

type screen int
type focusField int

const (
	modelScreen screen = iota
	provisionScreen
	translateScreen
)

const (
	sourceFocus focusField = iota
	targetFocus
	textFocus
	instructionFocus
	outputFocus
)

type model struct {
	screen        screen
	models        []models.QuantizedModel
	cursor        int
	selected      *models.QuantizedModel
	selectedName  string
	sourceLang    string
	targetLang    string
	sourceOptions []languages.Option
	targetOptions []languages.Option
	status        string
	output        string
	input         textarea.Model
	instruction   textarea.Model
	outputView    viewport.Model
	help          help.Model
	keys          keyMap
	service       *translate.Service
	runtime       *lf.Manager
	backendURL    string
	dataRoot      string
	streaming     bool

	windowWidth        int
	windowHeight       int
	startupRuntimePath string

	cfg   config.AppConfig
	state config.AppState

	focus focusField

	streamTask *taskState
	workTask   *taskState

	downloadPercent float64
	loadPercent     float64
	provisionStage  string
}

type streamDeltaMsg struct {
	Delta string
}

type streamDoneMsg struct {
	Final string
}

type streamErrMsg struct {
	Message string
}

type provisionProgressMsg struct {
	Stage   string
	Percent float64
	Message string
}

type provisionDoneMsg struct {
	ModelPath string
	Message   string
}

type provisionErrMsg struct {
	Message string
}

// Run starts the Bubble Tea app.
func Run(preselectedModelID, dataRoot string) error {
	m := newModel(preselectedModelID, dataRoot)
	defer func() {
		_ = m.runtime.Stop()
	}()
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(preselectedModelID, dataRoot string) model {
	in := newTextarea("Paste text to translate", 4000, 8)
	instruction := newTextarea("Optional translation instruction", 280, 4)
	instruction.Blur()

	all := huggingface.ListTranslateGemmaModels()
	cfg, _ := config.LoadAppConfig(dataRoot)
	state, _ := config.LoadAppState(dataRoot)
	backendURL := runtime.NormalizeBackendURL(state.BackendURL)
	if strings.TrimSpace(state.BackendURL) == "" {
		backendURL = runtime.NormalizeBackendURL(runtime.DefaultBackendURL)
	}

	activeID := strings.TrimSpace(preselectedModelID)
	if activeID == "" {
		activeID = strings.TrimSpace(cfg.ActiveModelID)
	}

	helpModel := help.New()
	helpModel.ShowAll = false
	helpModel.ShortSeparator = " | "

	m := model{
		screen:          modelScreen,
		models:          all,
		sourceLang:      "en",
		targetLang:      "zh-CN",
		sourceOptions:   languages.WithoutAuto(),
		targetOptions:   languages.WithoutAuto(),
		status:          "Scanning downloaded runtimes...",
		input:           in,
		instruction:     instruction,
		outputView:      viewport.New(80, 10),
		help:            helpModel,
		keys:            defaultKeyMap(),
		service:         translate.NewService(backendURL),
		runtime:         lf.NewManager(dataRoot, backendURL),
		backendURL:      backendURL,
		dataRoot:        dataRoot,
		windowWidth:     110,
		windowHeight:    34,
		cfg:             cfg,
		state:           state,
		focus:           textFocus,
		downloadPercent: -1,
		loadPercent:     -1,
	}
	m.applyLayout()

	if len(all) > 0 {
		for i := range all {
			if all[i].ID == activeID {
				m.cursor = i
				break
			}
		}
	}

	if m.bootstrapInstalledRuntime(activeID) {
		m.syncOutputViewport()
		return m
	}

	if probe := runtime.ProbeBackend(backendURL); probe.Ready {
		m.status = probe.Message
	} else {
		m.status = "No local runtime detected. Choose a model to install."
	}
	m.syncOutputViewport()
	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink}
	if m.startupRuntimePath != "" {
		cmds = append(cmds, startActivateRuntimeCmd(m.dataRoot, m.runtime, m.state, m.startupRuntimePath))
	}
	return tea.Batch(cmds...)
}

func newTextarea(placeholder string, charLimit, height int) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.CharLimit = charLimit
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.EndOfBufferCharacter = ' '
	ta.SetWidth(80)
	ta.SetHeight(height)

	focused, blurred := textarea.DefaultStyles()
	focused.Base = focused.Base.BorderStyle(asciiBorder).BorderForeground(colorAccent).Padding(0, 1)
	focused.CursorLine = lipgloss.NewStyle()
	blurred.Base = blurred.Base.BorderStyle(asciiBorder).BorderForeground(colorMutedBorder).Padding(0, 1)
	blurred.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle = focused
	ta.BlurredStyle = blurred
	_ = ta.Focus()
	return ta
}

func (m model) localModelPath(fileName string) string {
	return modelstore.LocalModelPath(m.dataRoot, fileName)
}

func (m *model) bootstrapInstalledRuntime(preferredID string) bool {
	catalog := modelstore.Catalog(m.dataRoot, m.models, strings.TrimSpace(preferredID), strings.TrimSpace(m.state.ActiveModelPath))

	if idx, item, ok := matchCatalogByPath(catalog, m.state.ActiveModelPath); ok {
		m.cursor = idx
		return m.prepareStartupRuntime(m.setKnownSelection(item, item.Path))
	}
	if idx, item, ok := matchCatalogByID(catalog, preferredID); ok {
		m.cursor = idx
		return m.prepareStartupRuntime(m.setKnownSelection(item, item.Path))
	}
	for idx, item := range catalog {
		if !item.Installed {
			continue
		}
		m.cursor = idx
		return m.prepareStartupRuntime(m.setKnownSelection(item, item.Path))
	}

	return false
}

func (m *model) setKnownSelection(item modelstore.CatalogItem, modelPath string) string {
	selected := item.QuantizedModel
	m.selected = &selected
	m.selectedName = selected.FileName
	m.cfg.ActiveModelID = selected.ID
	_ = config.SaveAppConfig(m.dataRoot, m.cfg)
	return m.applyRuntimePath(modelPath)
}

func (m *model) prepareStartupRuntime(modelPath string) bool {
	if strings.TrimSpace(modelPath) == "" {
		return false
	}
	if probe := runtime.ProbeBackend(m.backendURL); probe.Ready {
		m.screen = translateScreen
		m.status = probe.Message
		return true
	}

	m.screen = provisionScreen
	m.provisionStage = "load"
	m.downloadPercent = -1
	m.loadPercent = 0
	m.status = fmt.Sprintf("Starting runtime: %s", m.currentRuntimeName())
	m.startupRuntimePath = modelPath
	return true
}

func (m *model) applyRuntimePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	m.state.ActiveModelPath = path
	m.state.RuntimeMode = runtimeModeForPath(path)
	m.runtime.SetPreferredModelPath(path)
	_ = config.SaveAppState(m.dataRoot, m.state)
	return path
}

func matchCatalogByPath(items []modelstore.CatalogItem, targetPath string) (int, modelstore.CatalogItem, bool) {
	for idx, item := range items {
		if item.Path == "" || !sameRuntimePath(item.Path, targetPath) {
			continue
		}
		return idx, item, true
	}
	return 0, modelstore.CatalogItem{}, false
}

func matchCatalogByID(items []modelstore.CatalogItem, targetID string) (int, modelstore.CatalogItem, bool) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return 0, modelstore.CatalogItem{}, false
	}
	for idx, item := range items {
		if item.ID != targetID || !item.Installed {
			continue
		}
		return idx, item, true
	}
	return 0, modelstore.CatalogItem{}, false
}

func sameRuntimePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	return runtimeStem(a) == runtimeStem(b)
}

func runtimeStem(path string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	for _, suffix := range []string{".llamafile.exe", ".llamafile", ".exe"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return base
}

func runtimeModeForPath(path string) string {
	if strings.Contains(strings.ToLower(path), ".llamafile") {
		return "single_file_llamafile"
	}
	return ""
}
