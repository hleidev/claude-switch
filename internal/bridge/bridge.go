// Package bridge manages the local OpenAI-to-Anthropic protocol bridge daemon.
package bridge

import (
	"bytes"
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

// IsRunning reports whether the bridge daemon on port responds to /health.
func IsRunning(port int) bool {
	if port <= 0 {
		port = DefaultPort
	}
	client := http.Client{Timeout: 300 * time.Millisecond}
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
	if IsRunning(port) {
		return nil
	}

	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("uv is required to run the OpenAI bridge (install with: brew install uv)")
	}

	script, err := WriteScript()
	if err != nil {
		return fmt.Errorf("write bridge script: %w", err)
	}

	if foreground {
		cmd := exec.Command(uvPath, "run", script)
		cmd.Env = append(os.Environ(), fmt.Sprintf("BRIDGE_PORT=%d", port))
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
	cmd.Env = append(os.Environ(), fmt.Sprintf("BRIDGE_PORT=%d", port))
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bridge process: %w", err)
	}

	pidFile, err := pidPath()
	if err == nil {
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
	}

	// Poll until ready or timeout (up to 10s)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if IsRunning(port) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("bridge started (pid %d) but health check on :%d timed out; check %s", cmd.Process.Pid, port, logFile)
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
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Signal(syscall.SIGTERM)
				_ = os.Remove(pidFile)
			}
		}
	}

	// Double check if still listening on default port
	if IsRunning(DefaultPort) {
		// Wait shortly for graceful exit
		time.Sleep(200 * time.Millisecond)
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
	return running, pid, nil
}
