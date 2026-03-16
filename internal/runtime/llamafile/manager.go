package llamafile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"translategemma-ui/internal/runtime"
)

type launchCandidate struct {
	Name string
	Path string
	Args []string
}

// Progress represents runtime loading progress.
type Progress struct {
	Stage   string
	Percent float64
	Message string
}

// Manager starts and probes local llamafile runtime processes.
type Manager struct {
	DataRoot   string
	BackendURL string
	LogFile    string
	PidFile    string

	mu    sync.Mutex
	cfgMu sync.RWMutex
	cmd   *exec.Cmd

	ownedPID int

	preferredModelPath string
}

// NewManager creates a runtime manager bound to the local app data root.
func NewManager(dataRoot, backendURL string) *Manager {
	return &Manager{
		DataRoot:   dataRoot,
		BackendURL: runtime.NormalizeBackendURL(backendURL),
		LogFile:    filepath.Join(dataRoot, "logs", "runtime.log"),
		PidFile:    filepath.Join(dataRoot, "runtime.pid"),
	}
}

// SetBackendURL updates the local backend URL used by probes and launch commands.
func (m *Manager) SetBackendURL(raw string) {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	m.BackendURL = runtime.NormalizeBackendURL(raw)
}

// CurrentBackendURL returns the currently configured local backend URL.
func (m *Manager) CurrentBackendURL() string {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.BackendURL
}

// RuntimeStatus reports whether a managed runtime process recorded by this app is reachable.
func (m *Manager) RuntimeStatus() runtime.Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtimeStatusLocked()
}

// SetPreferredModelPath sets the exact packaged runtime path to launch first.
func (m *Manager) SetPreferredModelPath(path string) {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	m.preferredModelPath = strings.TrimSpace(path)
}

// Stop terminates the managed runtime process if it was started by this manager.
func (m *Manager) Stop() error {
	m.mu.Lock()
	var proc *os.Process
	pid := 0
	if m.cmd != nil && m.cmd.Process != nil {
		proc = m.cmd.Process
		pid = proc.Pid
		m.cmd = nil
	}
	m.mu.Unlock()

	if proc == nil {
		var err error
		pid, err = m.readManagedPID()
		if err != nil || pid <= 0 {
			return nil
		}
		proc, err = os.FindProcess(pid)
		if err != nil {
			_ = os.Remove(m.PidFile)
			return nil
		}
	}

	killErr := killManagedProcess(proc)
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		return killErr
	}
	m.clearOwnedPID()
	_ = os.Remove(m.PidFile)

	if waitManagedProcessExit(pid, 5*time.Second) {
		return nil
	}
	return nil
}

// StopOwned terminates the runtime only if it was started by this manager instance.
func (m *Manager) StopOwned() error {
	m.mu.Lock()
	ownedPID := m.ownedPID
	var proc *os.Process
	pid := ownedPID
	if ownedPID <= 0 {
		m.mu.Unlock()
		return nil
	}
	if m.cmd != nil && m.cmd.Process != nil && m.cmd.Process.Pid == ownedPID {
		proc = m.cmd.Process
		m.cmd = nil
	}
	m.mu.Unlock()

	if proc == nil {
		var err error
		pid, err = m.readManagedPID()
		if err != nil || pid <= 0 || pid != ownedPID {
			m.clearOwnedPID()
			return nil
		}
		foundProc, err := os.FindProcess(pid)
		if err != nil {
			m.clearOwnedPID()
			_ = os.Remove(m.PidFile)
			return nil
		}
		proc = foundProc
	}

	killErr := killManagedProcess(proc)
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		return killErr
	}
	m.clearOwnedPID()
	_ = os.Remove(m.PidFile)

	if waitManagedProcessExit(pid, 5*time.Second) {
		return nil
	}
	return nil
}

// EnsureRunning starts the runtime if backend health checks fail.
func (m *Manager) EnsureRunning() (runtime.Status, error) {
	return m.EnsureRunningWithProgress(nil)
}

// EnsureRunningWithProgress starts the runtime and emits coarse loading progress.
func (m *Manager) EnsureRunningWithProgress(onProgress func(Progress)) (runtime.Status, error) {
	reportProgress(onProgress, Progress{Stage: "load", Percent: 0, Message: "Checking runtime status"})
	backendURL := m.CurrentBackendURL()
	if status := m.RuntimeStatus(); status.Ready {
		reportProgress(onProgress, Progress{Stage: "load", Percent: 100, Message: "Runtime already ready"})
		return status, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	backendURL = m.CurrentBackendURL()
	if status := m.runtimeStatusLocked(); status.Ready {
		reportProgress(onProgress, Progress{Stage: "load", Percent: 100, Message: "Runtime already ready"})
		return status, nil
	}

	resolvedBackendURL, switchedPort, err := runtime.ResolveLaunchBackendURL(backendURL)
	if err != nil {
		return runtime.Status{Ready: false, Message: err.Error()}, err
	}
	if switchedPort {
		_, oldPort, _ := runtime.BackendAddress(backendURL)
		_, newPort, _ := runtime.BackendAddress(resolvedBackendURL)
		reportProgress(onProgress, Progress{
			Stage:   "load",
			Percent: 2,
			Message: fmt.Sprintf("Port %s is in use; switching runtime to %s", oldPort, newPort),
		})
	}
	m.SetBackendURL(resolvedBackendURL)

	host, port, err := runtime.BackendAddress(resolvedBackendURL)
	if err != nil {
		return runtime.Status{Ready: false, Message: "invalid backend URL"}, err
	}

	runtimePath, err := m.findRuntimeBinary()
	if err != nil {
		return runtime.Status{Ready: false, Message: err.Error()}, err
	}
	reportProgress(onProgress, Progress{Stage: "load", Percent: 5, Message: "Preparing runtime process"})
	launch, err := m.buildLaunchCommand(runtimePath, host, port)
	if err != nil {
		return runtime.Status{Ready: false, Message: err.Error()}, err
	}

	logf, err := os.OpenFile(m.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return runtime.Status{Ready: false, Message: "unable to open runtime log file"}, err
	}

	if fileExists(launch.Path) {
		_ = os.Chmod(launch.Path, 0o755)
	}
	cmd := exec.Command(launch.Path, launch.Args...)
	prepareLaunchCommand(cmd)
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		return runtime.Status{Ready: false, Message: "failed to launch runtime"}, fmt.Errorf("%s: %w", launch.Name, err)
	}
	m.cmd = cmd
	m.ownedPID = cmd.Process.Pid
	_ = os.WriteFile(m.PidFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644)

	go func(localCmd *exec.Cmd, localLog *os.File) {
		_ = localCmd.Wait()
		_ = localLog.Close()
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.cmd == localCmd {
			m.cmd = nil
		}
		if m.ownedPID == localCmd.Process.Pid {
			m.ownedPID = 0
		}
		pid, err := m.readManagedPID()
		if err == nil && pid == localCmd.Process.Pid {
			_ = os.Remove(m.PidFile)
		}
	}(cmd, logf)

	deadline := time.Now().Add(30 * time.Second)
	for {
		now := time.Now()
		if !now.Before(deadline) {
			break
		}
		status := runtime.ProbeBackend(m.CurrentBackendURL())
		if status.Ready {
			reportProgress(onProgress, Progress{Stage: "load", Percent: 100, Message: "Runtime ready"})
			return status, nil
		}
		elapsed := now.Sub(deadline.Add(-30 * time.Second))
		percent := 10 + (float64(elapsed)/float64(30*time.Second))*85
		if percent > 95 {
			percent = 95
		}
		reportProgress(onProgress, Progress{Stage: "load", Percent: percent, Message: "Loading model into runtime"})
		time.Sleep(500 * time.Millisecond)
	}
	return runtime.Status{Ready: false, Message: "runtime started but backend is still unreachable"}, fmt.Errorf("backend not reachable after launch; inspect %s", m.LogFile)
}

func (m *Manager) findRuntimeBinary() (string, error) {
	if preferred := m.preferredPath(); preferred != "" {
		if fileExists(preferred) && strings.Contains(strings.ToLower(preferred), ".llamafile") {
			return preferred, nil
		}
	}

	patterns := []string{
		filepath.Join(m.DataRoot, "runtimes", "*.llamafile*"),
	}
	var candidates []string
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		candidates = append(candidates, matches...)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no local runtime found in %s; download a packaged .llamafile from the model catalog first", filepath.Join(m.DataRoot, "runtimes"))
	}
	sort.Slice(candidates, func(i, j int) bool {
		ri, rj := candidateRank(candidates[i]), candidateRank(candidates[j])
		if ri != rj {
			return ri < rj
		}
		return candidates[i] < candidates[j]
	})
	return candidates[0], nil
}

func (m *Manager) buildLaunchCommand(runtimePath, host, port string) (launchCandidate, error) {
	serverArgs := []string{"--server", "--host", host, "--port", port}
	lower := strings.ToLower(runtimePath)
	if !strings.Contains(lower, ".llamafile") {
		return launchCandidate{}, fmt.Errorf("unsupported runtime format %q; install a packaged .llamafile from the model catalog", filepath.Base(runtimePath))
	}

	if shellPath, ok := shellWrapper(runtimePath); ok {
		return launchCandidate{
			Name: "direct runtime via shell",
			Path: shellPath,
			Args: append([]string{runtimePath}, serverArgs...),
		}, nil
	}

	return launchCandidate{
		Name: "direct runtime",
		Path: runtimePath,
		Args: serverArgs,
	}, nil
}

func candidateRank(path string) int {
	if strings.Contains(strings.ToLower(path), ".llamafile") {
		return 0
	}
	return 1
}

func (m *Manager) readManagedPID() (int, error) {
	raw, err := os.ReadFile(m.PidFile)
	if err != nil {
		return 0, err
	}
	pidText := strings.TrimSpace(string(raw))
	if pidText == "" {
		return 0, errors.New("empty runtime pid")
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func (m *Manager) runtimeStatusLocked() runtime.Status {
	pid, fromPIDFile := m.managedPIDLocked()
	if pid <= 0 {
		return runtime.Status{Ready: false, Message: "managed runtime is not running"}
	}
	if !managedProcessAlive(pid) {
		m.clearManagedReferenceLocked(pid, fromPIDFile)
		return runtime.Status{Ready: false, Message: "managed runtime is not running"}
	}
	status := runtime.ProbeBackend(m.CurrentBackendURL())
	if status.Ready {
		return status
	}
	return runtime.Status{
		Ready:   false,
		Message: fmt.Sprintf("managed runtime process %d is running but backend is not ready at %s", pid, m.CurrentBackendURL()),
	}
}

func (m *Manager) managedPIDLocked() (int, bool) {
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid, false
	}
	if m.ownedPID > 0 {
		return m.ownedPID, false
	}
	pid, err := m.readManagedPID()
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func (m *Manager) clearManagedReferenceLocked(pid int, fromPIDFile bool) {
	if m.cmd != nil && m.cmd.Process != nil && m.cmd.Process.Pid == pid {
		m.cmd = nil
	}
	if m.ownedPID == pid {
		m.ownedPID = 0
	}
	if fromPIDFile {
		_ = os.Remove(m.PidFile)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isExecutableFile(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !st.Mode().IsRegular() {
		return false
	}
	return st.Mode()&0o111 != 0
}

func reportProgress(cb func(Progress), p Progress) {
	if cb == nil {
		return
	}
	cb(p)
}

func (m *Manager) clearOwnedPID() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ownedPID = 0
}

func (m *Manager) preferredPath() string {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.preferredModelPath
}

func shellWrapper(runtimePath string) (string, bool) {
	if runtimePath == "" || !isAPEShellExecutable(runtimePath) {
		return "", false
	}
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		return "", false
	}
	return shellPath, true
}

func isAPEShellExecutable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 8)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false
	}
	if n < 8 {
		return false
	}
	return string(buf) == "MZqFpD='"
}
