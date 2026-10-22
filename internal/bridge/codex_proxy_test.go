package bridge

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestCodexProxyStatusNotRunning(t *testing.T) {
	running, pid, err := CodexProxyStatus(59999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running || pid != 0 {
		t.Errorf("expected not running, got running=%v, pid=%d", running, pid)
	}
}

func TestCodexProxyStatusMockHealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	parts := strings.Split(ts.URL, ":")
	portStr := parts[len(parts)-1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}

	running, pid, err := CodexProxyStatus(port)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !running || pid <= 0 {
		t.Errorf("expected running=true with pid, got running=%v, pid=%d", running, pid)
	}
}

func TestResolveCodexProxyCmd(t *testing.T) {
	exe, args, err := resolveCodexProxyCmd(8787)
	if err != nil {
		t.Logf("resolveCodexProxyCmd error (allowed in bare CI): %v", err)
		return
	}
	if exe == "" {
		t.Errorf("expected non-empty exe")
	}
	if len(args) == 0 {
		t.Errorf("expected non-empty args")
	}
}

func TestCodexProxyPaths(t *testing.T) {
	pid, err := codexProxyPIDPath(8787)
	if err != nil || !strings.HasSuffix(pid, "codex-proxy.pid") {
		t.Errorf("unexpected pid path: %v, %s", err, pid)
	}
	log, err := codexProxyLogPath(8787)
	if err != nil || !strings.HasSuffix(log, "codex-proxy.log") {
		t.Errorf("unexpected log path: %v, %s", err, log)
	}
	lock, err := codexProxyStartLockPath(8787)
	if err != nil || !strings.HasSuffix(lock, "codex-proxy.start.lock") {
		t.Errorf("unexpected lock path: %v, %s", err, lock)
	}

	// Non-default port
	pidCustom, _ := codexProxyPIDPath(9999)
	if !strings.HasSuffix(pidCustom, "codex-proxy.9999.pid") {
		t.Errorf("unexpected custom pid path: %s", pidCustom)
	}
}
