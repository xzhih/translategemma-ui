package tui

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
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
	screen         screen
	models         []models.QuantizedModel
	catalog        []modelstore.CatalogItem
	cursor         int
	selected       *models.QuantizedModel
	selectedName   string
	sourceLang     string
	targetLang     string
	sourceOptions  []languages.Option
	targetOptions  []languages.Option
	status         string
	output         string
	input          textarea.Model
	instruction    textarea.Model
	outputView     viewport.Model
	modelList      list.Model
	activity       spinner.Model
	downloadBar    progress.Model
	loadBar        progress.Model
	help           help.Model
	keys           keyMap
	service        *translate.Service
	runtime        *lf.Manager
	backendURL     string
	dataRoot       string
	streaming      bool
	runtimeReady   bool
	pendingRequest *translate.Request

	windowWidth  int
	windowHeight int

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
	ModelPath  string
	BackendURL string
	Message    string
}

type provisionErrMsg struct {
	Message string
}

// Run starts the Bubble Tea app.
func Run(preselectedModelID, dataRoot string) error {
	m := newModel(preselectedModelID, dataRoot)
	finalModel, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	runtimeManager := m.runtime
	if typedModel, ok := finalModel.(model); ok && typedModel.runtime != nil {
		runtimeManager = typedModel.runtime
	}
	stopErr := runtimeManager.StopOwned()
	if err != nil && stopErr != nil {
		return errors.Join(err, stopErr)
	}
	if err != nil {
		return err
	}
	return stopErr
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

	runtimeList := newRuntimeCatalogList()
	activity := spinner.New(spinner.WithSpinner(spinner.Line))
	activity.Style = lipgloss.NewStyle().Foreground(colorAccent)

	m := model{
		screen:        modelScreen,
		models:        all,
		sourceLang:    "en",
		targetLang:    "zh-CN",
		sourceOptions: languages.WithoutAuto(),
		targetOptions: languages.WithoutAuto(),
		status:        "Scanning downloaded runtimes...",
		input:         in,
		instruction:   instruction,
		outputView:    viewport.New(80, 10),
		modelList:     runtimeList,
		activity:      activity,
		downloadBar: progress.New(
			progress.WithSolidFill(string(colorAccent)),
			progress.WithFillCharacters('=', '-'),
			progress.WithoutPercentage(),
		),
		loadBar: progress.New(
			progress.WithSolidFill(string(colorSuccess)),
			progress.WithFillCharacters('=', '-'),
			progress.WithoutPercentage(),
		),
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
		m.syncCatalogList()
		m.syncOutputViewport()
		return m
	}

	if probe := runtime.ProbeBackend(backendURL); probe.Ready {
		m.runtimeReady = true
		m.status = probe.Message
	} else {
		m.runtimeReady = false
		m.status = "No local runtime detected. Choose a model to install."
	}
	m.syncCatalogList()
	m.syncOutputViewport()
	return m
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
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
	m.screen = translateScreen
	if probe := runtime.ProbeBackend(m.backendURL); probe.Ready {
		m.runtimeReady = true
		m.status = probe.Message
		return true
	}
	m.runtimeReady = false
	m.provisionStage = ""
	m.downloadPercent = -1
	m.loadPercent = -1
	m.status = "Runtime idle. The selected model will load on first translation."
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

func (m *model) syncBackendURL(next string, persist bool) {
	next = runtime.NormalizeBackendURL(next)
	if next == "" {
		next = runtime.NormalizeBackendURL(runtime.DefaultBackendURL)
	}
	m.backendURL = next
	m.state.BackendURL = next
	m.runtime.SetBackendURL(next)
	if m.service != nil {
		m.service.SetBackendURL(next)
	}
	if persist {
		_ = config.SaveAppState(m.dataRoot, m.state)
	}
}

func (m *model) resolveActiveRuntimePath() string {
	catalog := modelstore.Catalog(m.dataRoot, m.models, strings.TrimSpace(m.cfg.ActiveModelID), strings.TrimSpace(m.state.ActiveModelPath))

	if idx, item, ok := matchCatalogByPath(catalog, m.state.ActiveModelPath); ok {
		m.cursor = idx
		return m.setKnownSelection(item, item.Path)
	}
	if m.selected != nil {
		if idx, item, ok := matchCatalogByID(catalog, m.selected.ID); ok {
			m.cursor = idx
			return m.setKnownSelection(item, item.Path)
		}
	}
	if idx, item, ok := matchCatalogByID(catalog, m.cfg.ActiveModelID); ok {
		m.cursor = idx
		return m.setKnownSelection(item, item.Path)
	}
	if m.cursor >= 0 && m.cursor < len(catalog) {
		item := catalog[m.cursor]
		if item.Installed {
			return m.setKnownSelection(item, item.Path)
		}
	}
	for idx, item := range catalog {
		if !item.Installed {
			continue
		}
		m.cursor = idx
		return m.setKnownSelection(item, item.Path)
	}
	return ""
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
