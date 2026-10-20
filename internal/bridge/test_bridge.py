import asyncio
import json
import threading
import unittest
from unittest.mock import AsyncMock, patch

import litellm
from fastapi.testclient import TestClient

import bridge


class BridgeContractTests(unittest.TestCase):
    def setUp(self):
        self.client = TestClient(bridge.app)
        self.body = {
            "model": "gpt-6.1-sol[1m]",
            "messages": [{"role": "user", "content": "Reply OK."}],
            "max_tokens": 16,
            "stream": False,
        }

    def request(self, path="/https/example.com/v1/messages", custom_body=None):
        return self.client.post(
            path,
            headers={"x-api-key": "test-token"},
            json=custom_body if custom_body is not None else self.body,
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

    def test_request_without_stream_field_returns_json_message(self):
        completion = litellm.ModelResponse(
            model="gpt-6.1-sol",
            choices=[{"index": 0, "message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
            usage={"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
        )
        del self.body["stream"]
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock, return_value=completion) as call:
            response = self.request()
        self.assertFalse(call.call_args.kwargs["stream"])
        self.assertEqual(response.headers["content-type"], "application/json")
        self.assertEqual(response.json()["usage"]["input_tokens"], 3)

    def test_stop_sequences_automatically_hydrated_to_openai_stop(self):
        completion = litellm.ModelResponse(
            model="gpt-6.1-sol",
            choices=[{"index": 0, "message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
            usage={"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
        )
        self.body["stop_sequences"] = ["</block>", "\nObservation:"]
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock, return_value=completion) as call:
            response = self.request()
        self.assertEqual(response.status_code, 200)
        self.assertEqual(call.call_args.kwargs["stop"], ["</block>", "\nObservation:"])

    def test_thinking_adaptive_hydrated_to_upstream(self):
        completion = litellm.ModelResponse(
            model="gpt-6.1-sol",
            choices=[{"index": 0, "message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
            usage={"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
        )
        self.body["thinking"] = {"type": "adaptive"}
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock, return_value=completion) as call:
            response = self.request()
        self.assertEqual(response.status_code, 200)
        # Verify that Anthropic adaptive thinking is semantically translated to OpenAI reasoning_effort
        self.assertIn("reasoning_effort", call.call_args.kwargs)
        self.assertEqual(call.call_args.kwargs["reasoning_effort"], "medium")

    def test_long_tool_name_truncated_and_restored_in_response(self):
        long_name = "a_very_long_tool_name_that_exceeds_openai_sixty_four_character_limit_for_function_names"
        self.body["tools"] = [{
            "name": long_name,
            "description": "test tool",
            "input_schema": {"type": "object", "properties": {"q": {"type": "string"}}},
        }]
        
        # Upstream returns response with the truncated tool name
        # The adapter generates a truncated name ending in hash
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock) as call:
            # First, check what tool name is dispatched to acompletion
            def mock_acompletion(**kwargs):
                dispatched_tools = kwargs.get("tools", [])
                truncated_name = dispatched_tools[0]["function"]["name"]
                # Simulate OpenAI returning the truncated tool call
                mock_resp = litellm.ModelResponse(
                    model="gpt-6.1-sol",
                    choices=[{
                        "index": 0,
                        "message": {
                            "role": "assistant",
                            "content": None,
                            "tool_calls": [{
                                "id": "call_123",
                                "type": "function",
                                "function": {"name": truncated_name, "arguments": '{"q":"val"}'}
                            }]
                        },
                        "finish_reason": "tool_calls"
                    }],
                    usage={"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10},
                )
                return mock_resp

            call.side_effect = mock_acompletion
            response = self.request()

        self.assertEqual(response.status_code, 200)
        tool_use = response.json()["content"][0]
        self.assertEqual(tool_use["type"], "tool_use")
        # Assert that the client receives the RESTORED original long tool name!
        self.assertEqual(tool_use["name"], long_name)

    def test_usage_invariant_guard_populates_missing_tokens(self):
        # Simulate an upstream response completely lacking a usage block
        completion = litellm.ModelResponse(
            model="gpt-6.1-sol",
            choices=[{"index": 0, "message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
            usage=None,
        )
        with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock, return_value=completion):
            response = self.request()
        self.assertEqual(response.status_code, 200)
        data = response.json()
        self.assertIn("usage", data)
        self.assertEqual(data["usage"]["input_tokens"], 0)
        self.assertEqual(data["usage"]["output_tokens"], 0)

    def test_count_tokens_endpoint(self):
        body = {
            "model": "gpt-6.1-sol",
            "messages": [{"role": "user", "content": "Hello Claude! How are you?"}]
        }
        response = self.request(path="/https/example.com/v1/messages/count_tokens", custom_body=body)
        self.assertEqual(response.status_code, 200)
        data = response.json()
        self.assertIn("input_tokens", data)
        self.assertGreater(data["input_tokens"], 0)

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
        self.assertEqual(call.call_args.kwargs["num_retries"], 2)
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


class BridgeTransportResilienceTests(unittest.IsolatedAsyncioTestCase):
    async def test_pre_emit_transparent_retry_on_stream(self):
        """Verify that an initial connection drop during PRE_EMIT is transparently retried."""
        attempts = 0

        async def valid_stream():
            yield b'event: message_start\ndata: {"type": "message_start"}\n\n'
            yield b'event: content_block_delta\ndata: {"delta": {"text": "hello"}}\n\n'
            yield b'event: message_stop\ndata: {"type": "message_stop"}\n\n'

        async def flaky_acompletion(**kwargs):
            nonlocal attempts
            attempts += 1
            if attempts == 1:
                # First attempt fails with transient connection reset before headers are sent
                raise litellm.APIConnectionError(
                    message="Server disconnected (injected transient fault)",
                    model="gpt-6.1-sol",
                    llm_provider="openai",
                )
            return AsyncMock()

        import httpx

        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=bridge.app), base_url="http://test"
        ) as client:
            with patch.object(bridge.litellm, "acompletion", side_effect=flaky_acompletion), \
                 patch.object(bridge.adapter, "translate_completion_output_params_streaming", side_effect=lambda *args, **kwargs: valid_stream()):

                response = await client.post(
                    "/https/example.com/v1/messages",
                    headers={"x-api-key": "test-token"},
                    json={"model": "test", "messages": [{"role": "user", "content": "hi"}], "stream": True},
                )
                self.assertEqual(response.status_code, 200)
                self.assertIn("message_start", response.text)
                self.assertIn("message_stop", response.text)
                self.assertEqual(attempts, 2, "Must have retried transparently once")

    async def test_pre_emit_transparent_retry_on_non_stream(self):
        """Verify that a transient 503 during non-stream PRE_EMIT is transparently retried."""
        attempts = 0

        completion = litellm.ModelResponse(
            model="gpt-6.1-sol",
            choices=[{"index": 0, "message": {"role": "assistant", "content": "retried_ok"}, "finish_reason": "stop"}],
            usage={"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10},
        )

        async def flaky_acompletion(**kwargs):
            nonlocal attempts
            attempts += 1
            if attempts == 1:
                raise litellm.ServiceUnavailableError(
                    message="Upstream 503 transient error",
                    model="gpt-6.1-sol",
                    llm_provider="openai",
                )
            return completion

        import httpx

        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=bridge.app), base_url="http://test"
        ) as client:
            with patch.object(bridge.litellm, "acompletion", side_effect=flaky_acompletion):
                response = await client.post(
                    "/https/example.com/v1/messages",
                    headers={"x-api-key": "test-token"},
                    json={"model": "test", "messages": [{"role": "user", "content": "hi"}], "stream": False},
                )
                self.assertEqual(response.status_code, 200)
                self.assertEqual(response.json()["content"][0]["text"], "retried_ok")
                self.assertEqual(attempts, 2)

    async def test_post_emit_mid_stream_graceful_degradation(self):
        """Verify that mid-stream truncation gracefully emits an Anthropic error event and closes."""
        async def truncated_stream():
            yield b'event: message_start\ndata: {"type": "message_start"}\n\n'
            # Injected mid-stream connection drop
            raise litellm.APIConnectionError(
                message="Response payload is not completed: Not enough data to satisfy transfer length header.",
                model="gpt-6.1-sol",
                llm_provider="openai",
            )

        import httpx

        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=bridge.app), base_url="http://test"
        ) as client:
            with patch.object(bridge.litellm, "acompletion", new_callable=AsyncMock), \
                 patch.object(bridge.adapter, "translate_completion_output_params_streaming", return_value=truncated_stream()), \
                 self.assertLogs(bridge.logger, level="ERROR"):

                response = await client.post(
                    "/https/example.com/v1/messages",
                    headers={"x-api-key": "test-token"},
                    json={"model": "test", "messages": [{"role": "user", "content": "hi"}], "stream": True},
                )
                self.assertEqual(response.status_code, 200)
                self.assertIn("event: message_start", response.text)
                self.assertIn("event: error", response.text)
                self.assertIn("api_error", response.text)


if __name__ == "__main__":
    unittest.main()
