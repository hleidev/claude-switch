// Package bridge manages the local OpenAI-to-Anthropic protocol bridge daemon.
package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
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

// Bridge lifecycle states
const (
	StateReady    = "READY"
	StateDraining = "DRAINING"
)

// ProbeResult contains health probe information from a running bridge daemon.
type ProbeResult struct {
	Running        bool   `json:"running"`
	Hash           string `json:"hash"`
	State          string `json:"state"`
	PID            int    `json:"pid"`
	ActiveRequests int    `json:"active_requests"`
	SupportsDrain  bool   `json:"supports_drain"`
}

// ScriptHash returns the SHA-256 hash of the embedded bridge.py script.
func ScriptHash() string {
	h := sha256.Sum256(Script)
	return hex.EncodeToString(h[:])
}

// Probe inspects the bridge daemon on the specified port.
func Probe(port int) (*ProbeResult, error) {
	if port <= 0 {
		port = DefaultPort
	}
	client := http.Client{Timeout: 1 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	resp, err := client.Get(url)
	if err != nil {
		return &ProbeResult{Running: false}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &ProbeResult{Running: false}, nil
	}

	var data struct {
		Status         string `json:"status"`
		Hash           string `json:"hash"`
		State          string `json:"state"`
		PID            int    `json:"pid"`
		ActiveRequests int    `json:"active_requests"`
		SupportsDrain  bool   `json:"supports_drain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return &ProbeResult{Running: true, State: StateReady}, nil
	}

	state := data.State
	if state == "" {
		state = StateReady
	}

	return &ProbeResult{
		Running:        true,
		Hash:           data.Hash,
		State:          state,
		PID:            data.PID,
		ActiveRequests: data.ActiveRequests,
		SupportsDrain:  data.SupportsDrain,
	}, nil
}

// Drain instructs the running bridge daemon to gracefully retire and stop accepting new traffic.
func Drain(port int, timeout time.Duration) error {
	if port <= 0 {
		port = DefaultPort
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	client := http.Client{Timeout: timeout}
	url := fmt.Sprintf("http://127.0.0.1:%d/control/drain", port)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("drain endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

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

func pidPath(ports ...int) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if len(ports) > 0 && ports[0] > 0 && ports[0] != DefaultPort {
		return filepath.Join(dir, fmt.Sprintf("bridge.%d.pid", ports[0])), nil
	}
	return filepath.Join(dir, "bridge.pid"), nil
}

func logPath(ports ...int) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if len(ports) > 0 && ports[0] > 0 && ports[0] != DefaultPort {
		return filepath.Join(dir, fmt.Sprintf("bridge.%d.log", ports[0])), nil
	}
	return filepath.Join(dir, "bridge.log"), nil
}

func startLockPath(ports ...int) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if len(ports) > 0 && ports[0] > 0 && ports[0] != DefaultPort {
		return filepath.Join(dir, fmt.Sprintf("bridge.%d.start.lock", ports[0])), nil
	}
	return filepath.Join(dir, "bridge.start.lock"), nil
}

func daemonLockPath(ports ...int) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if len(ports) > 0 && ports[0] > 0 && ports[0] != DefaultPort {
		return filepath.Join(dir, fmt.Sprintf("bridge.%d.daemon.lock", ports[0])), nil
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

// IsRunning reports whether the bridge daemon on port responds to /health and is in READY state.
func IsRunning(port int) bool {
	probe, _ := Probe(port)
	return probe != nil && probe.Running && probe.State == StateReady
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

// EnsureRunning checks if the bridge is running with the latest script;
// if not running or outdated, performs a zero-downtime hot-reload or fresh start.
func EnsureRunning() error {
	return EnsureUpToDate(DefaultPort)
}

// EnsureUpToDate ensures the bridge on port is healthy and matches the current binary's embedded script.
func EnsureUpToDate(port int) error {
	if port <= 0 {
		port = DefaultPort
	}

	lockFile, err := startLockPath(port)
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

	probe, _ := Probe(port)
	expectedHash := ScriptHash()

	if probe != nil && probe.Running {
		// If running, hash matches, and state is READY, nothing to do
		if probe.Hash == expectedHash && probe.State == StateReady {
			return nil
		}

		// Version drift or not READY: perform graceful drain handoff
		if probe.SupportsDrain {
			_ = Drain(port, 1*time.Second)
			for i := 0; i < 20; i++ {
				time.Sleep(50 * time.Millisecond)
				if p, _ := Probe(port); p == nil || !p.Running {
					break
				}
			}
		} else {
			// Legacy daemon without drain endpoint -> clean stop
			_ = Stop(port)
			time.Sleep(100 * time.Millisecond)
		}
	}

	return startLocked(false, port)
}

// Restart gracefully drains any running bridge daemon and starts a new one.
func Restart(foreground bool, port int) error {
	if port <= 0 {
		port = DefaultPort
	}

	lockFile, err := startLockPath(port)
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

	probe, _ := Probe(port)
	if probe != nil && probe.Running {
		if probe.SupportsDrain {
			_ = Drain(port, 1*time.Second)
			for i := 0; i < 20; i++ {
				time.Sleep(50 * time.Millisecond)
				if p, _ := Probe(port); p == nil || !p.Running {
					break
				}
			}
		} else {
			_ = Stop(port)
			time.Sleep(100 * time.Millisecond)
		}
	}

	return startLocked(foreground, port)
}

// Start launches the bridge either in the background or foreground.
func Start(foreground bool, port int) error {
	if port <= 0 {
		port = DefaultPort
	}

	// Use file lock to prevent concurrent Start races across multiple terminals
	lockFile, err := startLockPath(port)
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

	// If already healthy and matching expected hash, nothing to do
	probe, _ := Probe(port)
	if probe != nil && probe.Running && probe.Hash == ScriptHash() && probe.State == StateReady {
		return nil
	}

	return startLocked(foreground, port)
}

func startLocked(foreground bool, port int) error {
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

	bridgeDaemonLock, _ := daemonLockPath(port)

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

	logFile, err := logPath(port)
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

	pidFile, err := pidPath(port)
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
			probe, _ := Probe(port)
			if probe != nil && probe.Running && probe.State == StateReady {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("bridge started (pid %d) but health check on :%d timed out; check %s", cmd.Process.Pid, port, logFile)
			}
		}
	}
}

// Stop terminates any running bridge daemon.
func Stop(ports ...int) error {
	port := DefaultPort
	if len(ports) > 0 && ports[0] > 0 {
		port = ports[0]
	}

	pidFile, err := pidPath(port)
	if err == nil {
		data, rerr := os.ReadFile(pidFile)
		if rerr == nil {
			pidStr := strings.TrimSpace(string(data))
			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
				terminatePID(pid, 2*time.Second)
				_ = os.Remove(pidFile)
			}
		}
	}

	// Double check if target port is still occupied (orphaned child or untracked process)
	if listeningPID := findListeningPID(port); listeningPID > 0 {
		terminatePID(listeningPID, 2*time.Second)
	}

	return nil
}

// ExtendedStatus returns detailed health probe information including version hash and state.
func ExtendedStatus(port int) (*ProbeResult, error) {
	if port <= 0 {
		port = DefaultPort
	}
	return Probe(port)
}

// Status returns whether the bridge is running, its recorded pid, and any error.
func Status(port int) (bool, int, error) {
	if port <= 0 {
		port = DefaultPort
	}
	probe, _ := Probe(port)
	running := probe != nil && probe.Running && probe.State == StateReady
	pid := 0
	if probe != nil && probe.PID > 0 {
		pid = probe.PID
	} else {
		pidFile, err := pidPath(port)
		if err == nil {
			if data, rerr := os.ReadFile(pidFile); rerr == nil {
				pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			}
		}
	}

	// If running, ensure the PID is alive or sync with listening socket PID
	if running {
		if pid <= 0 || syscall.Kill(pid, 0) != nil {
			if realPID := findListeningPID(port); realPID > 0 {
				pid = realPID
				if pidFile, err := pidPath(port); err == nil && pidFile != "" {
					_ = os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0o600)
				}
			}
		}
	}
	return running, pid, nil
}
