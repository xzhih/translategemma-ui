package tui

import (
	"errors"
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
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/runtimeutil"
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
	downloadedBytes int64
	downloadTotal   int64
	downloadSpeed   float64
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
	Stage            string
	Percent          float64
	Downloaded       int64
	Total            int64
	SpeedBytesPerSec float64
	Message          string
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
	in := newTextarea("Source text", 4000, 8)
	in.ShowLineNumbers = true
	instruction := newTextarea("Optional instruction", 280, 4)
	instruction.Blur()

	all := huggingface.ListTranslateGemmaModels()
	cfg, _ := config.LoadAppConfig(dataRoot)
	state, _ := config.LoadAppState(dataRoot)
	backendURL := runtimeutil.SyncBackendURL(&state, state.BackendURL)

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
		windowWidth:     80,
		windowHeight:    24,
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

	if status := m.runtime.RuntimeStatus(); status.Ready {
		m.runtimeReady = true
		m.status = status.Message
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
	focused.Base = focused.Base.Padding(0, 0)
	focused.CursorLine = lipgloss.NewStyle()
	blurred.Base = blurred.Base.Padding(0, 0)
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

	if idx, item, ok := modelstore.ResolveCatalogItem(catalog, m.state.ActiveModelPath, modelstore.ResolveOptions{
		PreferTextRuntime: true,
	}, preferredID, m.cfg.ActiveModelID); ok {
		m.cursor = idx
		return m.prepareStartupRuntime(m.setKnownSelection(item, item.Path))
	}
	return false
}

func (m *model) setKnownSelection(item modelstore.CatalogItem, modelPath string) string {
	selected := item.QuantizedModel
	m.selected = &selected
	m.selectedName = selected.FileName
	runtimeutil.ApplyActiveModel(&m.cfg, &m.state, selected, modelPath)
	_ = config.SaveAppConfig(m.dataRoot, m.cfg)
	return m.applyRuntimePath(modelPath)
}

func (m *model) prepareStartupRuntime(modelPath string) bool {
	if strings.TrimSpace(modelPath) == "" {
		return false
	}
	m.screen = translateScreen
	if status := m.runtime.RuntimeStatus(); status.Ready {
		m.runtimeReady = true
		m.status = status.Message
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
	runtimeutil.ApplyRuntimePath(&m.state, path)
	m.runtime.SetPreferredModelPath(path)
	_ = config.SaveAppState(m.dataRoot, m.state)
	return path
}

func (m *model) syncBackendURL(next string, persist bool) {
	m.backendURL = runtimeutil.SyncBackendURL(&m.state, next, m.runtime, m.service)
	if persist {
		_ = config.SaveAppState(m.dataRoot, m.state)
	}
}

func (m *model) resolveActiveRuntimePath() string {
	catalog := modelstore.Catalog(m.dataRoot, m.models, strings.TrimSpace(m.cfg.ActiveModelID), strings.TrimSpace(m.state.ActiveModelPath))

	preferredIDs := make([]string, 0, 2)
	if m.selected != nil {
		preferredIDs = append(preferredIDs, m.selected.ID)
	}
	preferredIDs = append(preferredIDs, m.cfg.ActiveModelID)
	if idx, item, ok := modelstore.ResolveCatalogItem(catalog, m.state.ActiveModelPath, modelstore.ResolveOptions{
		PreferTextRuntime: true,
	}, preferredIDs...); ok {
		m.cursor = idx
		return m.setKnownSelection(item, item.Path)
	}
	return ""
}
