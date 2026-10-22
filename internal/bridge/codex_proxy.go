package bridge

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// DefaultCodexProxyPort is the default port for the local codex-openai-proxy daemon.
const DefaultCodexProxyPort = 8787

func codexProxyPIDPath(ports ...int) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if len(ports) > 0 && ports[0] > 0 && ports[0] != DefaultCodexProxyPort {
		return filepath.Join(dir, fmt.Sprintf("codex-proxy.%d.pid", ports[0])), nil
	}
	return filepath.Join(dir, "codex-proxy.pid"), nil
}

func codexProxyLogPath(ports ...int) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if len(ports) > 0 && ports[0] > 0 && ports[0] != DefaultCodexProxyPort {
		return filepath.Join(dir, fmt.Sprintf("codex-proxy.%d.log", ports[0])), nil
	}
	return filepath.Join(dir, "codex-proxy.log"), nil
}

func codexProxyStartLockPath(ports ...int) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if len(ports) > 0 && ports[0] > 0 && ports[0] != DefaultCodexProxyPort {
		return filepath.Join(dir, fmt.Sprintf("codex-proxy.%d.start.lock", ports[0])), nil
	}
	return filepath.Join(dir, "codex-proxy.start.lock"), nil
}

// CodexProxyStatus checks if the codex-openai-proxy daemon is listening on the given port and healthy.
func CodexProxyStatus(ports ...int) (running bool, pid int, err error) {
	port := DefaultCodexProxyPort
	if len(ports) > 0 && ports[0] > 0 {
		port = ports[0]
	}

	pid = findListeningPID(port)
	if pid <= 0 {
		return false, 0, nil
	}

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(healthURL)
	if err != nil {
		return false, pid, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, pid, nil
	}
	return false, pid, nil
}

// resolveCodexProxyCmd locates an executable runner for codex-openai-proxy.
func resolveCodexProxyCmd(port int) (string, []string, error) {
	home, err := os.UserHomeDir()
	if err == nil {
		// Priority 1: Isolated local bundle inside claude-switch config directory
		localProxyCli := filepath.Join(home, ".config", "claude-switch", "proxy", "dist", "cli.js")
		if _, err := os.Stat(localProxyCli); err == nil {
			node22Matches, _ := filepath.Glob(filepath.Join(home, ".local/share/mise/installs/node/22*/bin/node"))
			if len(node22Matches) > 0 {
				return node22Matches[len(node22Matches)-1], []string{localProxyCli, "--port", strconv.Itoa(port)}, nil
			}
			if p, err := exec.LookPath("node"); err == nil {
				return p, []string{"--experimental-websocket", localProxyCli, "--port", strconv.Itoa(port)}, nil
			}
		}

		localBinScript := filepath.Join(home, ".local", "bin", "codex-proxy")
		if info, err := os.Stat(localBinScript); err == nil && (info.Mode()&0111 != 0) {
			return localBinScript, []string{"--port", strconv.Itoa(port)}, nil
		}
	}

	if p, err := exec.LookPath("codex-proxy"); err == nil {
		return p, []string{"--port", strconv.Itoa(port)}, nil
	}
	if p, err := exec.LookPath("codex-openai-proxy"); err == nil {
		return p, []string{"--port", strconv.Itoa(port)}, nil
	}

	if home != "" {
		node22Matches, _ := filepath.Glob(filepath.Join(home, ".local/share/mise/installs/node/22*/bin/node"))
		proxyMatches, _ := filepath.Glob(filepath.Join(home, ".local/share/mise/installs/node/*/bin/codex-openai-proxy"))
		if len(node22Matches) > 0 && len(proxyMatches) > 0 {
			return node22Matches[0], []string{proxyMatches[0], "--port", strconv.Itoa(port)}, nil
		}
	}

	if npxPath, err := exec.LookPath("npx"); err == nil {
		return npxPath, []string{"@thkdog/codex-openai-proxy", "--port", strconv.Itoa(port)}, nil
	}

	return "", nil, fmt.Errorf("neither codex-proxy nor npx found in PATH")
}

// EnsureCodexProxyRunning guarantees that codex-openai-proxy is up and healthy on the given port.
func EnsureCodexProxyRunning(ports ...int) error {
	port := DefaultCodexProxyPort
	if len(ports) > 0 && ports[0] > 0 {
		port = ports[0]
	}

	running, _, _ := CodexProxyStatus(port)
	if running {
		return nil
	}

	lockPath, err := codexProxyStartLockPath(port)
	if err != nil {
		return err
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("acquire codex proxy start lock file: %w", err)
	}
	defer lockFile.Close()

	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("flock codex proxy start lock: %w", err)
	}
	defer unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)

	running, _, _ = CodexProxyStatus(port)
	if running {
		return nil
	}

	// Terminate stale listener if it's dead/unresponsive
	if pid := findListeningPID(port); pid > 0 {
		terminatePID(pid, 2*time.Second)
	}

	exe, args, err := resolveCodexProxyCmd(port)
	if err != nil {
		return fmt.Errorf("locate codex-proxy executable: %w", err)
	}

	logFile, err := codexProxyLogPath(port)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open codex-proxy log: %w", err)
	}
	defer out.Close()

	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start codex-proxy process: %w", err)
	}

	if pidPath, err := codexProxyPIDPath(port); err == nil {
		_ = os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
	}

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := http.Client{Timeout: 500 * time.Millisecond}
	start := time.Now()
	for time.Since(start) < 5*time.Second {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("codex-proxy started (pid %d) but health check failed on :%d within 5s (check %s)", cmd.Process.Pid, port, logFile)
}

// StopCodexProxy stops the codex-openai-proxy daemon on the given port.
func StopCodexProxy(ports ...int) error {
	port := DefaultCodexProxyPort
	if len(ports) > 0 && ports[0] > 0 {
		port = ports[0]
	}

	pid := findListeningPID(port)
	if pidPath, err := codexProxyPIDPath(port); err == nil {
		if data, err := os.ReadFile(pidPath); err == nil {
			if filePID, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && filePID > 0 {
				if pid <= 0 {
					pid = filePID
				}
			}
		}
		_ = os.Remove(pidPath)
	}

	if pid > 0 {
		terminatePID(pid, 3*time.Second)
	}
	return nil
}
