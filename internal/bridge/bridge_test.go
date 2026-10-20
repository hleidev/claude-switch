package bridge

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestIsRunning(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Extract port
	addr := ts.Listener.Addr().(*net.TCPAddr)
	port := addr.Port

	if !IsRunning(port) {
		t.Fatalf("expected IsRunning(%d) to be true for healthy test server", port)
	}

	// Non-listening port
	if IsRunning(port + 1000) {
		t.Fatalf("expected IsRunning to be false for non-listening port")
	}
}

func TestReadRecentLogs(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "bridge_test_log_*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	logs := readRecentLogs(tmpFile.Name(), 2)
	expected := "line 4 | line 5"
	if logs != expected {
		t.Fatalf("expected %q, got %q", expected, logs)
	}
}

func TestTerminatePID_SelfTerminatingChild(t *testing.T) {
	// Start a dummy sleeper process to test terminatePID
	proc, err := os.StartProcess("/bin/sleep", []string{"sleep", "60"}, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	pid := proc.Pid
	// Verify alive
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("expected child %d to be alive", pid)
	}

	terminatePID(pid, 2*time.Second)

	// In Unix, direct child process must be waited on to reap zombie
	state, err := proc.Wait()
	if err != nil {
		t.Fatalf("proc.Wait() error: %v", err)
	}
	if state.Success() {
		t.Fatalf("expected child to be terminated, not exit cleanly")
	}
}

func TestStatusSelfHealingPID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	addr := ts.Listener.Addr().(*net.TCPAddr)
	port := addr.Port

	if !IsRunning(port) {
		t.Fatalf("test server must respond to IsRunning")
	}

	running, _, err := Status(port)
	if err != nil {
		t.Fatal(err)
	}
	if !running {
		t.Fatalf("expected running to be true")
	}
}

func TestScriptHash(t *testing.T) {
	h := ScriptHash()
	if len(h) != 64 {
		t.Fatalf("expected 64-char hex hash, got %q (len %d)", h, len(h))
	}
}

func TestProbe_MetadataParsing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"status": "ok",
				"service": "claude-switch-bridge",
				"hash": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				"state": "READY",
				"active_requests": 2,
				"pid": 12345,
				"supports_drain": true
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	port := ts.Listener.Addr().(*net.TCPAddr).Port

	probe, err := Probe(port)
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if !probe.Running {
		t.Fatalf("expected probe.Running to be true")
	}
	if probe.Hash != "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Fatalf("unexpected hash: %s", probe.Hash)
	}
	if probe.State != StateReady {
		t.Fatalf("expected state READY, got %s", probe.State)
	}
	if probe.ActiveRequests != 2 {
		t.Fatalf("expected 2 active requests, got %d", probe.ActiveRequests)
	}
	if probe.PID != 12345 {
		t.Fatalf("expected PID 12345, got %d", probe.PID)
	}
	if !probe.SupportsDrain {
		t.Fatalf("expected SupportsDrain to be true")
	}
}

func TestProbe_DrainingState_NotReady(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok", "state": "DRAINING", "supports_drain": true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	port := ts.Listener.Addr().(*net.TCPAddr).Port

	probe, err := Probe(port)
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if !probe.Running {
		t.Fatalf("expected probe.Running to be true")
	}
	if probe.State != StateDraining {
		t.Fatalf("expected state DRAINING, got %s", probe.State)
	}

	// IsRunning must report false when daemon is in DRAINING state
	if IsRunning(port) {
		t.Fatalf("expected IsRunning to be false when daemon is DRAINING")
	}
}

func TestDrain_Call(t *testing.T) {
	drained := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/control/drain" && r.Method == http.MethodPost {
			drained = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "draining"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	port := ts.Listener.Addr().(*net.TCPAddr).Port

	if err := Drain(port, 1*time.Second); err != nil {
		t.Fatalf("Drain error: %v", err)
	}
	if !drained {
		t.Fatalf("expected drain endpoint to be called")
	}
}
