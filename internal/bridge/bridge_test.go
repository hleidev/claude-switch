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
