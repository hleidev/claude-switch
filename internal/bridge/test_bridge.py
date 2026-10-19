import asyncio
import json
import threading
import unittest
from unittest.mock import AsyncMock, patch

import litellm
from fastapi.testclient import TestClient

import bridge


class BridgeErrorTests(unittest.TestCase):
    def setUp(self):
        self.client = TestClient(bridge.app)
        self.body = {
            "model": "gpt-6.1-sol[1m]",
            "messages": [{"role": "user", "content": "Reply OK."}],
            "max_tokens": 16,
            "stream": False,
        }

    def request(self):
        return self.client.post(
            "/https/example.com/v1/messages",
            headers={"x-api-key": "test-token"},
            json=self.body,
        )

    def test_upstream_errors_preserve_status_and_anthropic_error_type(self):
        for exception, status, error_type in [
            (litellm.BadRequestError, 400, "invalid_request_error"),
            (litellm.AuthenticationError, 401, "authentication_error"),
            (litellm.NotFoundError, 404, "not_found_error"),
            (litellm.RateLimitError, 429, "rate_limit_error"),
        ]:
            with self.subTest(status=status):
                error = exception(
                    message="Upstream rejected request",
                    model="gpt-6.1-sol",
                    llm_provider="openai",
                )
                with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock, side_effect=error), \
                        self.assertLogs(bridge.logger, level="ERROR"):
                    response = self.request()
                self.assertEqual(response.status_code, status)
                self.assertEqual(response.json()["type"], "error")
                self.assertEqual(response.json()["error"]["type"], error_type)
                self.assertIn("Upstream rejected request", response.json()["error"]["message"])

    def test_unexpected_error_remains_500(self):
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock, side_effect=RuntimeError("broken")), \
                self.assertLogs(bridge.logger, level="ERROR"):
            response = self.request()
        self.assertEqual(response.status_code, 500)
        self.assertEqual(response.json()["error"]["type"], "api_error")

    def test_stream_failure_emits_anthropic_error_event(self):
        async def chunks():
            yield b'event: message_start\ndata: {}\n\n'
            raise litellm.NotFoundError(
                message="Model not found", model="gpt-6.1-sol", llm_provider="openai"
            )

        self.body["stream"] = True
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock) as acompletion, \
                patch.object(bridge.adapter, "translate_completion_output_params_streaming", return_value=chunks()), \
                self.assertLogs(bridge.logger, level="ERROR"):
            response = self.request()
        self.assertEqual(acompletion.call_args.kwargs["stream_options"], {"include_usage": True})
        self.assertEqual(response.status_code, 200)
        self.assertIn("event: message_start", response.text)
        self.assertIn("event: error", response.text)
        payload = json.loads(response.text.split("event: error\ndata: ")[1])
        self.assertEqual(payload["error"]["type"], "not_found_error")

    def test_success_keeps_selected_model(self):
        completion = litellm.ModelResponse(
            model="gpt-6.1-sol",
            choices=[{"index": 0, "message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
            usage={"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
        )
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock, return_value=completion) as call:
            response = self.request()
        self.assertEqual(response.status_code, 200)
        self.assertEqual(call.call_args.kwargs["model"], "openai/gpt-6.1-sol")
        self.assertEqual(response.json()["model"], "gpt-6.1-sol[1m]")
        self.assertEqual(response.json()["content"][0]["text"], "OK")


class BridgeConcurrencyTests(unittest.IsolatedAsyncioTestCase):
    async def test_health_responds_while_upstream_completion_is_blocked(self):
        started = asyncio.Event()
        release = asyncio.Event()

        async def blocked_completion(**kwargs):
            started.set()
            await release.wait()
            raise RuntimeError("upstream released")

        import httpx

        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=bridge.app), base_url="http://test"
        ) as client:
            with patch.object(bridge.litellm, "acompletion", side_effect=blocked_completion):
                request = asyncio.create_task(client.post(
                    "/https/example.com/v1/messages",
                    headers={"x-api-key": "test-token"},
                    json={"model": "test", "messages": [{"role": "user", "content": "OK"}], "stream": False},
                ))
                try:
                    await asyncio.wait_for(started.wait(), timeout=1.0)
                    self.assertFalse(request.done(), "completion must still be waiting")
                    response = await asyncio.wait_for(client.get("/health"), timeout=0.1)
                    self.assertEqual(response.status_code, 200)
                    self.assertFalse(request.done(), "health must respond before completion finishes")
                finally:
                    release.set()
                    await request

    async def test_health_responds_while_stream_is_actively_slow(self):
        chunk_event = asyncio.Event()
        finish_event = asyncio.Event()

        async def slow_stream():
            yield b'event: message_start\ndata: {}\n\n'
            chunk_event.set()
            await finish_event.wait()
            yield b'event: message_stop\ndata: {}\n\n'

        import httpx

        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=bridge.app), base_url="http://test"
        ) as client:
            with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock), \
                 patch.object(bridge.adapter, "translate_completion_output_params_streaming", return_value=slow_stream()):
                
                async def consume_stream():
                    async with client.stream(
                        "POST",
                        "/https/example.com/v1/messages",
                        headers={"x-api-key": "test-token"},
                        json={"model": "test", "messages": [{"role": "user", "content": "OK"}], "stream": True},
                    ) as resp:
                        async for _ in resp.aiter_bytes():
                            pass

                stream_task = asyncio.create_task(consume_stream())
                try:
                    await asyncio.wait_for(chunk_event.wait(), timeout=1.0)
                    # While stream is waiting, health check must respond fast!
                    response = await asyncio.wait_for(client.get("/health"), timeout=0.05)
                    self.assertEqual(response.status_code, 200)
                    self.assertFalse(stream_task.done())
                finally:
                    finish_event.set()
                    await stream_task


if __name__ == "__main__":
    unittest.main()
