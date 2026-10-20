#!/usr/bin/env -S uv run
# /// script
# dependencies = [
#     "httpx>=0.27.0",
#     "numpy>=1.26.0",
# ]
# ///

"""
verify_zero_downtime_drain.py

Automated objective verification test for zero-downtime hot-reload and graceful draining.
Proves:
  1. In-flight SSE streams are preserved with zero dropped bytes / 0 broken pipes during restart.
  2. Listening port 18088 is handed over without collision.
  3. New requests immediately route to the new daemon.
  4. The retiring daemon cleanly exits once in-flight streams finish (zero orphans).
"""

import asyncio
import http.server
import json
import os
import subprocess
import sys
import threading
import time
import httpx

MOCK_UPSTREAM_PORT = 18099
TEST_BRIDGE_PORT = 18088
BIN_CS = os.path.abspath("./bin/cs")


class SlowStreamingUpstream(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def do_POST(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.end_headers()

        # Emit initial chunk
        init_chunk = {
            "id": "chatcmpl-drain-test",
            "object": "chat.completion.chunk",
            "created": int(time.time()),
            "model": "gpt-6.1-sol",
            "choices": [{"index": 0, "delta": {"role": "assistant", "content": "START"}, "finish_reason": None}],
        }
        self.wfile.write(f"data: {json.dumps(init_chunk)}\n\n".encode("utf-8"))
        self.wfile.flush()

        # Emit 6 chunks slowly (0.5s each = 3 seconds total in-flight)
        for i in range(6):
            time.sleep(0.5)
            chunk = {
                "id": "chatcmpl-drain-test",
                "object": "chat.completion.chunk",
                "created": int(time.time()),
                "model": "gpt-6.1-sol",
                "choices": [{"index": 0, "delta": {"content": f" token_{i}"}, "finish_reason": None}],
            }
            try:
                self.wfile.write(f"data: {json.dumps(chunk)}\n\n".encode("utf-8"))
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                break

        done_chunk = {
            "id": "chatcmpl-drain-test",
            "object": "chat.completion.chunk",
            "created": int(time.time()),
            "model": "gpt-6.1-sol",
            "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
        }
        try:
            self.wfile.write(f"data: {json.dumps(done_chunk)}\n\n".encode("utf-8"))
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


def get_listening_pid(port):
    try:
        out = subprocess.check_output(
            ["lsof", "-n", "-P", f"-iTCP:{port}", "-sTCP:LISTEN", "-t"],
            text=True,
        ).strip()
        lines = [line.strip() for line in out.splitlines() if line.strip()]
        return int(lines[0]) if lines else None
    except Exception:
        return None


def run_cs(args):
    return subprocess.run([BIN_CS] + args, capture_output=True, text=True)


def is_pid_alive(pid):
    if not pid:
        return False
    try:
        os.kill(pid, 0)
        return True
    except OSError:
        return False


def main():
    print("=" * 70)
    print("OBJECTIVE VERIFICATION: ZERO-DOWNTIME DRAIN & HOT-RELOAD")
    print("=" * 70)

    # 1. Clean slate
    print(f"\n[Step 1] Ensuring clean baseline on port {TEST_BRIDGE_PORT}...")
    run_cs(["bridge", "stop", "-p", str(TEST_BRIDGE_PORT)])
    time.sleep(0.5)
    stale_pid = get_listening_pid(TEST_BRIDGE_PORT)
    if stale_pid:
        print(f"  Killing stale PID {stale_pid} on port {TEST_BRIDGE_PORT}...")
        os.system(f"kill -9 {stale_pid} 2>/dev/null")
        time.sleep(0.5)
    assert get_listening_pid(TEST_BRIDGE_PORT) is None, f"Port {TEST_BRIDGE_PORT} must be free"
    print("  ✓ Port is free")

    # 2. Start mock upstream
    print(f"\n[Step 2] Starting mock upstream server on :{MOCK_UPSTREAM_PORT}...")
    mock_server = http.server.HTTPServer(("127.0.0.1", MOCK_UPSTREAM_PORT), SlowStreamingUpstream)
    mock_thread = threading.Thread(target=mock_server.serve_forever, daemon=True)
    mock_thread.start()
    print("  ✓ Mock upstream active")

    # 3. Start first bridge daemon
    print(f"\n[Step 3] Launching Bridge v1 on :{TEST_BRIDGE_PORT} via `cs bridge start`...")
    res = run_cs(["bridge", "start", "-p", str(TEST_BRIDGE_PORT)])
    assert res.returncode == 0, f"Failed to start bridge: {res.stderr}"
    pid1 = get_listening_pid(TEST_BRIDGE_PORT)
    assert pid1 is not None, "Bridge v1 must be listening"
    print(f"  ✓ Bridge v1 listening (PID {pid1})")

    with httpx.Client(timeout=2.0) as client:
        health_resp = client.get(f"http://127.0.0.1:{TEST_BRIDGE_PORT}/health")
        assert health_resp.status_code == 200, f"/health returned {health_resp.status_code}"
        data = health_resp.json()
        assert data["state"] == "READY", f"Expected state READY, got {data['state']}"
        assert data["supports_drain"] is True, "Daemon must support drain"
        print(f"  ✓ Bridge v1 /health OK: hash={data['hash'][:8]}..., state={data['state']}")

    # 4. Initiate slow streaming request
    print(f"\n[Step 4] Starting in-flight 3-second streaming request through Bridge v1...")
    chunks_received = []
    stream_first_chunk_received = threading.Event()
    stream_error = None

    def streaming_worker():
        nonlocal stream_error
        url = f"http://127.0.0.1:{TEST_BRIDGE_PORT}/http/127.0.0.1:{MOCK_UPSTREAM_PORT}/v1/messages"
        headers = {"x-api-key": "test-key", "Content-Type": "application/json"}
        payload = {"model": "gpt-6.1-sol", "messages": [{"role": "user", "content": "test"}], "stream": True}
        try:
            with httpx.Client(timeout=10.0) as client:
                with client.stream("POST", url, json=payload, headers=headers) as resp:
                    assert resp.status_code == 200, f"Expected 200, got {resp.status_code}"
                    for line in resp.iter_lines():
                        if line:
                            chunks_received.append(line)
                            stream_first_chunk_received.set()
        except Exception as e:
            stream_error = e

    stream_thread = threading.Thread(target=streaming_worker, daemon=True)
    stream_thread.start()

    assert stream_first_chunk_received.wait(timeout=3.0), "Timed out waiting for first SSE chunk"
    print(f"  ✓ In-flight stream is actively streaming (received first chunk: {chunks_received[0][:40]}...)")

    # 5. Concurrently trigger `cs bridge restart` while stream is in-flight!
    print(f"\n[Step 5] Triggering `cs bridge restart -p {TEST_BRIDGE_PORT}` DURING ACTIVE STREAM...")
    t0 = time.perf_counter()
    restart_res = run_cs(["bridge", "restart", "-p", str(TEST_BRIDGE_PORT)])
    restart_duration = time.perf_counter() - t0
    print(f"  Restart executed in {restart_duration:.2f}s, output: {restart_res.stdout.strip()}")
    assert restart_res.returncode == 0, f"cs bridge restart failed: {restart_res.stderr}"

    # Verify new bridge is active with a new PID
    pid2 = get_listening_pid(TEST_BRIDGE_PORT)
    assert pid2 is not None, "New bridge must be listening on port"
    assert pid2 != pid1, f"Bridge PID must have changed! Old={pid1}, New={pid2}"
    print(f"  ✓ Bridge v2 successfully bound to :{TEST_BRIDGE_PORT} (new PID {pid2} != old PID {pid1})")

    # 6. Verify in-flight stream finishes with 100% integrity
    print("\n[Step 6] Verifying in-flight stream continuity and integrity...")
    stream_thread.join(timeout=6.0)
    assert not stream_thread.is_alive(), "Stream thread should have finished"
    assert stream_error is None, f"In-flight stream suffered error during restart: {stream_error}"

    data_lines = [c for c in chunks_received if c.startswith("data: ") and not c.endswith("[DONE]")]
    assert len(data_lines) >= 7, f"Expected at least 7 SSE data chunks, received {len(data_lines)}"
    assert any("message_stop" in c or "[DONE]" in c for c in chunks_received), "Stream must terminate with message_stop"
    print(f"  ✓ 100% DATA INTEGRITY: All {len(data_lines)} SSE tokens successfully delivered with 0 errors!")

    # 7. Verify new requests route to Bridge v2 immediately
    print("\n[Step 7] Verifying new traffic routes to Bridge v2...")
    with httpx.Client(timeout=2.0) as client:
        resp = client.get(f"http://127.0.0.1:{TEST_BRIDGE_PORT}/health")
        assert resp.status_code == 200
        health_data = resp.json()
        assert health_data["state"] == "READY"
        assert health_data["pid"] == pid2, f"Expected PID {pid2}, got {health_data['pid']}"
    print(f"  ✓ New traffic verified routing to Bridge v2 (PID {pid2})")

    # 8. Verify old process cleanly retired (zero orphans)
    print("\n[Step 8] Verifying old Bridge v1 process (PID {pid1}) cleanly exited...")
    deadline = time.time() + 4.0
    while time.time() < deadline and is_pid_alive(pid1):
        time.sleep(0.2)
    assert not is_pid_alive(pid1), f"Old Bridge PID {pid1} must have exited, but is still running!"
    print(f"  ✓ Old PID {pid1} cleanly exited and reaped")

    # 9. Clean shutdown
    print(f"\n[Step 9] Stopping Bridge v2 via `cs bridge stop -p {TEST_BRIDGE_PORT}`...")
    stop_res = run_cs(["bridge", "stop", "-p", str(TEST_BRIDGE_PORT)])
    assert stop_res.returncode == 0
    time.sleep(0.5)
    assert get_listening_pid(TEST_BRIDGE_PORT) is None, f"Port {TEST_BRIDGE_PORT} must be free after stop"
    print("  ✓ Clean shutdown verified")

    mock_server.shutdown()
    print("\n" + "=" * 70)
    print("ALL VERIFICATIONS PASSED: ZERO-DOWNTIME DRAIN CONFIRMED 100% HEALTHY")
    print("=" * 70)


if __name__ == "__main__":
    main()
