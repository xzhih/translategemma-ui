package web

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/languages"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/runtime"
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/translate"
)

const (
	maxHistory       = 200
	maxImageUploadMB = 10
)

var errNoLocalModel = errors.New("no local model installed; select one to download first")

type translationService interface {
	TranslateWithContext(context.Context, translate.Request) (string, error)
	StreamTranslateWithContextAndProgress(context.Context, translate.Request, func(string) error, func(translate.ProgressUpdate) error) (string, error)
	TranslateImageWithContext(context.Context, translate.ImageRequest) (string, error)
	SetBackendURL(string)
}

type runtimeController interface {
	SetBackendURL(string)
	SetPreferredModelPath(string)
	CurrentBackendURL() string
	Stop() error
	EnsureRunningWithProgress(func(lf.Progress)) (runtime.Status, error)
}

type uiStatus struct {
	Code    string
	Message string
}

// Server owns web handlers and in-memory UI state.
type Server struct {
	uiShell        []byte
	static         http.Handler
	translator     translationService
	runtimeManager runtimeController
	backendURL     string
	dataRoot       string

	availableModels []models.QuantizedModel
	activeModel     models.QuantizedModel
	languages       []languageOption

	probeBackend func(string) runtime.Status
	now          func() time.Time

	cfg   config.AppConfig
	state config.AppState

	mu            sync.Mutex
	installMu     sync.Mutex
	history       []historyItem
	nextHistoryID int64
	status        uiStatus
}

type historyItem struct {
	ID     int64
	Source string
	Target string
	Input  string
	Output string
	When   time.Time
}

type languageOption = languages.Option

type modelPayload struct {
	ID            string `json:"id"`
	FileName      string `json:"fileName"`
	Size          string `json:"size"`
	Installed     bool   `json:"installed"`
	Active        bool   `json:"active"`
	Selected      bool   `json:"selected"`
	Loaded        bool   `json:"loaded"`
	VisionCapable bool   `json:"visionCapable"`
	Recommended   bool   `json:"recommended"`
}

type historyPayload struct {
	ID     int64  `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Input  string `json:"input"`
	Output string `json:"output"`
	When   string `json:"when"`
}

type appStatePayload struct {
	PageTitle         string           `json:"pageTitle"`
	ActiveTab         string           `json:"activeTab"`
	TextSourceLang    string           `json:"textSourceLang"`
	TextTargetLang    string           `json:"textTargetLang"`
	TextInstruction   string           `json:"textInstruction"`
	TextInput         string           `json:"textInput"`
	TextOutput        string           `json:"textOutput"`
	FileSourceLang    string           `json:"fileSourceLang"`
	FileTargetLang    string           `json:"fileTargetLang"`
	FileInstruction   string           `json:"fileInstruction"`
	FileOutput        string           `json:"fileOutput"`
	Status            string           `json:"status"`
	StatusCode        string           `json:"statusCode,omitempty"`
	ActiveModelID     string           `json:"activeModelId"`
	Models            []modelPayload   `json:"models"`
	Languages         []languageOption `json:"languages"`
	History           []historyPayload `json:"history"`
	HistoryCount      int              `json:"historyCount"`
	RuntimeStatus     string           `json:"runtimeStatus"`
	RuntimeStatusCode string           `json:"runtimeStatusCode,omitempty"`
	RuntimeReady      bool             `json:"runtimeReady"`
	NeedsModelSetup   bool             `json:"needsModelSetup"`
	VisionEnabled     bool             `json:"visionEnabled"`
	MaxUploadMB       int              `json:"maxUploadMB"`
	Now               string           `json:"now"`
}

type streamEvent struct {
	Type                string          `json:"type"`
	Stage               string          `json:"stage,omitempty"`
	Message             string          `json:"message,omitempty"`
	MessageCode         string          `json:"messageCode,omitempty"`
	Percent             float64         `json:"percent,omitempty"`
	DownloadedBytes     int64           `json:"downloadedBytes,omitempty"`
	TotalBytes          int64           `json:"totalBytes,omitempty"`
	SpeedBytesPerSecond float64         `json:"speedBytesPerSecond,omitempty"`
	Delta               string          `json:"delta,omitempty"`
	Output              string          `json:"output,omitempty"`
	History             *historyPayload `json:"history,omitempty"`
	Count               int             `json:"count,omitempty"`
}

type imageResult struct {
	OK          bool            `json:"ok"`
	Output      string          `json:"output,omitempty"`
	Message     string          `json:"message,omitempty"`
	MessageCode string          `json:"messageCode,omitempty"`
	History     *historyPayload `json:"history,omitempty"`
	Count       int             `json:"count,omitempty"`
}

type historyDeleteResponse struct {
	OK         bool   `json:"ok"`
	HistoryID  int64  `json:"history_id,omitempty"`
	Count      int    `json:"count"`
	Status     string `json:"status"`
	StatusCode string `json:"statusCode,omitempty"`
}
