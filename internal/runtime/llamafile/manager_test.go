package llamafile

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBuildLaunchCommandLlamafileUsesServerArgs(t *testing.T) {
	dataRoot := t.TempDir()
	m := NewManager(dataRoot, "http://127.0.0.1:8080")

	candidate, err := m.buildLaunchCommand("/tmp/model.llamafile", "127.0.0.1", "8080")
	if err != nil {
		t.Fatalf("buildLaunchCommand returned error: %v", err)
	}
	args := strings.Join(candidate.Args, " ")
	if !strings.Contains(args, "--server") || !strings.Contains(args, "--host 127.0.0.1") || !strings.Contains(args, "--port 8080") {
		t.Fatalf("expected server launch args, got %q", args)
	}
}

func TestBuildLaunchCommandAPELLamafileUsesShellWrapper(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	dataRoot := t.TempDir()
	runtimePath := filepath.Join(dataRoot, "model.llamafile")
	if err := os.WriteFile(runtimePath, []byte("MZqFpD='fake-ape"), 0o755); err != nil {
		t.Fatalf("write fake runtime: %v", err)
	}

	m := NewManager(dataRoot, "http://127.0.0.1:8080")
	candidate, err := m.buildLaunchCommand(runtimePath, "127.0.0.1", "8080")
	if err != nil {
		t.Fatalf("buildLaunchCommand returned error: %v", err)
	}
	if candidate.Path != shellPath {
		t.Fatalf("expected shell wrapper %q, got %q", shellPath, candidate.Path)
	}
	if len(candidate.Args) == 0 || candidate.Args[0] != runtimePath {
		t.Fatalf("expected runtime path as first shell arg, got %#v", candidate.Args)
	}
}

func TestBuildLaunchCommandPlainLlamafileStaysDirect(t *testing.T) {
	dataRoot := t.TempDir()
	runtimePath := filepath.Join(dataRoot, "model.llamafile")
	if err := os.WriteFile(runtimePath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write fake runtime: %v", err)
	}

	m := NewManager(dataRoot, "http://127.0.0.1:8080")
	candidate, err := m.buildLaunchCommand(runtimePath, "127.0.0.1", "8080")
	if err != nil {
		t.Fatalf("buildLaunchCommand returned error: %v", err)
	}
	if candidate.Path != runtimePath {
		t.Fatalf("expected direct runtime path %q, got %q", runtimePath, candidate.Path)
	}
}

func TestBuildLaunchCommandRejectsNonLlamafile(t *testing.T) {
	m := NewManager(t.TempDir(), "http://127.0.0.1:8080")
	if _, err := m.buildLaunchCommand("/tmp/model.gguf", "127.0.0.1", "8080"); err == nil {
		t.Fatalf("expected non-llamafile runtime to be rejected")
	}
}

func TestStopOwnedKillsSessionProcess(t *testing.T) {
	dataRoot := t.TempDir()
	m := NewManager(dataRoot, "http://127.0.0.1:1")

	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	m.cmd = cmd
	m.ownedPID = cmd.Process.Pid
	if err := os.WriteFile(m.PidFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	if err := m.StopOwned(); err != nil {
		t.Fatalf("stop owned process: %v", err)
	}

	if m.ownedPID != 0 {
		t.Fatalf("expected owned pid to be cleared, got %d", m.ownedPID)
	}
	if _, err := os.Stat(m.PidFile); !os.IsNotExist(err) {
		t.Fatalf("expected pid file to be removed, got err=%v", err)
	}
}

func TestStopOwnedLeavesSharedRuntimeUntouched(t *testing.T) {
	dataRoot := t.TempDir()
	m := NewManager(dataRoot, "http://127.0.0.1:1")
	if err := os.WriteFile(m.PidFile, []byte("4242\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	if err := m.StopOwned(); err != nil {
		t.Fatalf("stop owned on shared pid file: %v", err)
	}

	if _, err := os.Stat(m.PidFile); err != nil {
		t.Fatalf("expected shared pid file to remain untouched, got err=%v", err)
	}
}

func TestStopOwnedKillsChildProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group signaling is unix-specific")
	}

	dataRoot := t.TempDir()
	m := NewManager(dataRoot, "http://127.0.0.1:1")
	childPIDFile := filepath.Join(dataRoot, "child.pid")
	script := fmt.Sprintf("sleep 30 & child=$!; echo $child > %s; wait $child", childPIDFile)

	cmd := exec.Command("sh", "-c", script)
	prepareLaunchCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start shell wrapper: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(childPIDFile); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	rawChildPID, err := os.ReadFile(childPIDFile)
	if err != nil {
		t.Fatalf("read child pid file: %v", err)
	}

	var childPID int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(rawChildPID)), "%d", &childPID); err != nil {
		t.Fatalf("parse child pid: %v", err)
	}

	m.cmd = cmd
	m.ownedPID = cmd.Process.Pid
	if err := os.WriteFile(m.PidFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	if err := m.StopOwned(); err != nil {
		t.Fatalf("stop owned process tree: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	if waitForUnixProcessExit(childPID, time.Until(deadline)) {
		return
	}
	t.Fatalf("expected child process %d to be terminated with its process group", childPID)
}

func waitForUnixProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
