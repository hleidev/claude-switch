// Package bridge manages the local OpenAI-to-Anthropic protocol bridge daemon.
package bridge

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hleidev/claude-switch/internal/config"
)

//go:embed bridge.py
var Script []byte

// DefaultPort is the default port for the local bridge daemon.
const DefaultPort = 8088

func configDir() (string, error) {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(cfgPath), nil
}

func scriptPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.py"), nil
}

func pidPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.pid"), nil
}

func logPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.log"), nil
}

func startLockPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.start.lock"), nil
}

func daemonLockPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.daemon.lock"), nil
}

func findListeningPID(port int) int {
	if port <= 0 {
		port = DefaultPort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lsof", "-n", "-P", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-t")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	pidStr := strings.TrimSpace(string(out))
	if lines := strings.Split(pidStr, "\n"); len(lines) > 0 {
		if pid, err := strconv.Atoi(strings.TrimSpace(lines[0])); err == nil {
			return pid
		}
	}
	return 0
}

func terminatePID(pid int, timeout time.Duration) {
	if pid <= 0 {
		return
	}
	// Try terminating process group first, fallback to single process
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Signal(syscall.SIGKILL)
	}
}

func readRecentLogs(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, " | ")
}

// IsRunning reports whether the bridge daemon on port responds to /health.
func IsRunning(port int) bool {
	if port <= 0 {
		port = DefaultPort
	}
	client := http.Client{Timeout: 1 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// WriteScript ensures bridge.py in ~/.config/claude-switch is up to date with embedded bytes.
func WriteScript() (string, error) {
	p, err := scriptPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	existing, err := os.ReadFile(p)
	if err == nil && bytes.Equal(existing, Script) {
		return p, nil
	}
	if err := os.WriteFile(p, Script, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// EnsureRunning checks if the bridge is running; if not, launches it in the background.
func EnsureRunning() error {
	if IsRunning(DefaultPort) {
		return nil
	}
	return Start(false, DefaultPort)
}

// Start launches the bridge either in the background or foreground.
func Start(foreground bool, port int) error {
	if port <= 0 {
		port = DefaultPort
	}

	// Use file lock to prevent concurrent Start races across multiple terminals
	lockFile, err := startLockPath()
	if err == nil {
		if lockDir := filepath.Dir(lockFile); lockDir != "" {
			_ = os.MkdirAll(lockDir, 0o700)
		}
		lf, lerr := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o600)
		if lerr == nil {
			defer lf.Close()
			if ferr := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); ferr == nil {
				defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
			}
		}
	}

	// If already healthy, nothing to do
	if IsRunning(port) {
		return nil
	}

	// If unhealthy but the port is occupied by a stale/zombie process, clean it up first
	if occupiedPID := findListeningPID(port); occupiedPID > 0 {
		terminatePID(occupiedPID, 2*time.Second)
		time.Sleep(100 * time.Millisecond)
	}

	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("uv is required to run the OpenAI bridge (install with: brew install uv)")
	}

	script, err := WriteScript()
	if err != nil {
		return fmt.Errorf("write bridge script: %w", err)
	}

	bridgeDaemonLock, _ := daemonLockPath()

	if foreground {
		cmd := exec.Command(uvPath, "run", script)
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("BRIDGE_PORT=%d", port),
			fmt.Sprintf("BRIDGE_LOCK_PATH=%s", bridgeDaemonLock),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	logFile, err := logPath()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open bridge log: %w", err)
	}
	defer out.Close()

	cmd := exec.Command(uvPath, "run", script)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("BRIDGE_PORT=%d", port),
		fmt.Sprintf("BRIDGE_LOCK_PATH=%s", bridgeDaemonLock),
	)
	// Keep the daemon alive when its launching terminal session exits.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bridge process: %w", err)
	}

	pidFile, err := pidPath()
	if err == nil {
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
	}

	// Responsive supervision: watch for premature child exit while polling health
	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
	}()

	deadline := time.Now().Add(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case exitErr := <-exitCh:
			recentLogs := readRecentLogs(logFile, 5)
			if recentLogs != "" {
				return fmt.Errorf("bridge process (pid %d) exited unexpectedly (%v): %s", cmd.Process.Pid, exitErr, recentLogs)
			}
			return fmt.Errorf("bridge process (pid %d) exited unexpectedly (%v); check %s", cmd.Process.Pid, exitErr, logFile)
		case <-ticker.C:
			if IsRunning(port) {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("bridge started (pid %d) but health check on :%d timed out; check %s", cmd.Process.Pid, port, logFile)
			}
		}
	}
}

// Stop terminates any running bridge daemon.
func Stop() error {
	pidFile, err := pidPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pidFile)
	if err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			terminatePID(pid, 2*time.Second)
			_ = os.Remove(pidFile)
		}
	}

	// Double check if default port is still occupied (orphaned child or untracked process)
	if listeningPID := findListeningPID(DefaultPort); listeningPID > 0 {
		terminatePID(listeningPID, 2*time.Second)
	}

	return nil
}

// Status returns whether the bridge is running, its recorded pid, and any error.
func Status(port int) (bool, int, error) {
	if port <= 0 {
		port = DefaultPort
	}
	running := IsRunning(port)
	pid := 0
	pidFile, err := pidPath()
	if err == nil {
		if data, rerr := os.ReadFile(pidFile); rerr == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
	}
	// If running, ensure the PID is alive or sync with listening socket PID
	if running {
		if pid <= 0 || syscall.Kill(pid, 0) != nil {
			if realPID := findListeningPID(port); realPID > 0 {
				pid = realPID
				if pidFile != "" {
					_ = os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0o600)
				}
			}
		}
	}
	return running, pid, nil
}
