package tui

import (
	"context"
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/huggingface"
	"translategemma-ui/internal/models"
	lf "translategemma-ui/internal/runtime/llamafile"
	"translategemma-ui/internal/translate"
)

type taskKind int

const (
	streamTaskKind taskKind = iota
	provisionTaskKind
)

type taskState struct {
	msgs     <-chan tea.Msg
	cancel   context.CancelFunc
	onCancel func()
}

type taskStartedMsg struct {
	kind taskKind
	task *taskState
}

type taskClosedMsg struct {
	kind taskKind
	task *taskState
}

func (t *taskState) stop() {
	if t == nil {
		return
	}
	if t.onCancel != nil {
		t.onCancel()
	}
	if t.cancel != nil {
		t.cancel()
	}
}

func startTaskCmd(kind taskKind, onCancel func(), runner func(context.Context, chan<- tea.Msg)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan tea.Msg, 128)
		go func() {
			defer close(ch)
			runner(ctx, ch)
		}()
		return taskStartedMsg{
			kind: kind,
			task: &taskState{
				msgs:     ch,
				cancel:   cancel,
				onCancel: onCancel,
			},
		}
	}
}

func waitForTaskCmd(kind taskKind, task *taskState) tea.Cmd {
	if task == nil || task.msgs == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-task.msgs
		if !ok {
			return taskClosedMsg{kind: kind, task: task}
		}
		return msg
	}
}

func sendTaskMsg(ctx context.Context, out chan<- tea.Msg, msg tea.Msg) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- msg:
		return nil
	}
}

func startStreamCmd(req translate.Request, svc *translate.Service) tea.Cmd {
	return startTaskCmd(streamTaskKind, nil, func(ctx context.Context, out chan<- tea.Msg) {
		final, err := svc.StreamTranslateWithContext(ctx, req, func(delta string) error {
			return sendTaskMsg(ctx, out, streamDeltaMsg{Delta: delta})
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			_ = sendTaskMsg(ctx, out, streamErrMsg{Message: err.Error()})
			return
		}
		_ = sendTaskMsg(ctx, out, streamDoneMsg{Final: final})
	})
}

func startInstallModelCmd(dataRoot string, selected models.QuantizedModel, runtimeManager *lf.Manager, state config.AppState) tea.Cmd {
	return startTaskCmd(provisionTaskKind, nil, func(ctx context.Context, out chan<- tea.Msg) {
		_ = sendTaskMsg(ctx, out, provisionProgressMsg{Stage: "download", Percent: 0, Message: "Downloading model"})
		modelPath, err := huggingface.DownloadModelWithContext(ctx, dataRoot, selected, func(p huggingface.DownloadProgress) {
			_ = sendTaskMsg(ctx, out, provisionProgressMsg{
				Stage:   "download",
				Percent: p.Percent,
				Message: p.Message,
			})
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			_ = sendTaskMsg(ctx, out, provisionErrMsg{Message: err.Error()})
			return
		}
		if ctx.Err() != nil {
			return
		}
		runActivateRuntimeTask(ctx, out, dataRoot, runtimeManager, state, modelPath)
	})
}

func startActivateRuntimeCmd(dataRoot string, runtimeManager *lf.Manager, state config.AppState, modelPath string) tea.Cmd {
	return startTaskCmd(provisionTaskKind, nil, func(ctx context.Context, out chan<- tea.Msg) {
		runActivateRuntimeTask(ctx, out, dataRoot, runtimeManager, state, modelPath)
	})
}

func runActivateRuntimeTask(ctx context.Context, out chan<- tea.Msg, dataRoot string, runtimeManager *lf.Manager, state config.AppState, modelPath string) {
	if ctx.Err() != nil {
		return
	}
	_ = sendTaskMsg(ctx, out, provisionProgressMsg{Stage: "load", Percent: 0, Message: "Preparing active runtime"})
	runtimeManager.SetPreferredModelPath(modelPath)
	state.ActiveModelPath = modelPath
	state.RuntimeMode = runtimeModeForPath(modelPath)
	_ = config.SaveAppState(dataRoot, state)

	if ctx.Err() != nil {
		return
	}
	_ = sendTaskMsg(ctx, out, provisionProgressMsg{Stage: "load", Percent: 0, Message: "Loading model into runtime"})
	_ = runtimeManager.Stop()
	status, err := runtimeManager.EnsureRunningWithProgress(func(p lf.Progress) {
		_ = sendTaskMsg(ctx, out, provisionProgressMsg{
			Stage:   p.Stage,
			Percent: p.Percent,
			Message: p.Message,
		})
	})
	if ctx.Err() != nil {
		_ = runtimeManager.Stop()
		return
	}
	if err != nil {
		_ = sendTaskMsg(ctx, out, provisionErrMsg{Message: err.Error()})
		return
	}
	state.BackendURL = runtimeManager.CurrentBackendURL()
	_ = config.SaveAppState(dataRoot, state)
	_ = sendTaskMsg(ctx, out, provisionProgressMsg{Stage: "load", Percent: 100, Message: status.Message})
	_ = sendTaskMsg(ctx, out, provisionDoneMsg{
		ModelPath:  strings.TrimSpace(modelPath),
		BackendURL: runtimeManager.CurrentBackendURL(),
		Message:    status.Message,
	})
}
