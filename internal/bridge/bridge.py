#!/usr/bin/env -S uv run
# /// script
# dependencies = [
#     "fastapi>=0.110.0",
#     "uvicorn>=0.28.0",
#     "litellm[proxy]>=1.40.0",
#     "httpx>=0.27.0",
# ]
# ///

import os
import re
import json
import logging
import asyncio
import random
from typing import Optional
from contextlib import asynccontextmanager
from fastapi import FastAPI, Request, Response, HTTPException
from fastapi.responses import StreamingResponse, JSONResponse
from starlette.concurrency import run_in_threadpool
import httpx
import uvicorn
import litellm
from litellm.llms.anthropic.pass_through.adapters.transformation import AnthropicAdapter

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("claude-bridge")

# Suppress noisy litellm telemetry / verbose logs
litellm.telemetry = False
litellm.drop_params = True

# Proactive Socket Hygiene:
# Bound keepalive expiry to 20s to proactively evict idle/half-open TCP sockets
# before upstream firewalls/proxies drop them mid-stream.
@asynccontextmanager
async def lifespan(app: FastAPI):
    if not getattr(litellm, "aclient_session", None) or litellm.aclient_session.is_closed:
        limits = httpx.Limits(max_keepalive_connections=20, max_connections=100, keepalive_expiry=20.0)
        timeout = httpx.Timeout(connect=15.0, read=300.0, write=15.0, pool=10.0)
        litellm.aclient_session = httpx.AsyncClient(limits=limits, timeout=timeout)
    yield
    if getattr(litellm, "aclient_session", None) and not litellm.aclient_session.is_closed:
        await litellm.aclient_session.aclose()

app = FastAPI(title="Claude Code OpenAI Bridge", lifespan=lifespan)
adapter = AnthropicAdapter()

DEFAULT_FALLBACK_MODEL = "gpt-6.1-sol"


async def async_next_chunk(aiter):
    """Safely fetch next chunk from an async iterator, returning None on StopAsyncIteration."""
    try:
        return await aiter.__anext__()
    except StopAsyncIteration:
        return None


def is_transient_error(error: Exception) -> bool:
    """Determine whether an error is safe to retry transparently before first chunk is emitted."""
    status = getattr(error, "status_code", None)
    if isinstance(status, int) and status in {500, 502, 503, 504, 408, 429}:
        return True
    msg = str(error).lower()
    transient_indicators = [
        "connection", "disconnected", "reset", "timeout", "timed out",
        "eof", "broken pipe", "transfer encoding", "cannot connect"
    ]
    return any(ind in msg for ind in transient_indicators)


def completion_error(error: Exception) -> tuple[int, dict]:
    status = getattr(error, "status_code", None)
    if not isinstance(status, int) or not 400 <= status <= 599:
        status = 500
    error_type = {
        400: "invalid_request_error",
        401: "authentication_error",
        403: "permission_error",
        404: "not_found_error",
        413: "request_too_large",
        422: "invalid_request_error",
        429: "rate_limit_error",
        529: "overloaded_error",
    }.get(status, "api_error")
    return status, {
        "type": "error",
        "error": {"type": error_type, "message": str(error)},
    }


def parse_upstream_target(path: str) -> tuple[str, str]:
    """
    Extracts the upstream base URL and the sub-endpoint from the incoming path.
    Supported path patterns:
      /https/<host>/<subpath>/v1/messages/count_tokens -> (https://<host>/<subpath>/v1, /v1/messages/count_tokens)
      /https/<host>/<subpath>/v1/messages              -> (https://<host>/<subpath>/v1, /v1/messages)
      /http/<host>/<subpath>/v1/messages               -> (http://<host>/<subpath>/v1, /v1/messages)
      /proxy/<url>/v1/messages                          -> (<url>, /v1/messages)
    """
    cleaned = path.lstrip("/")
    m = re.match(r"^(https?)/([^/]+)(/.*)?$", cleaned)
    if m:
        proto, host, rest = m.group(1), m.group(2), m.group(3) or ""
        # Match longest endpoints first to avoid premature suffix match
        for endpoint in ["/v1/messages/count_tokens", "/v1/messages", "/api/hello", "/v1/models"]:
            if rest.endswith(endpoint):
                base_sub = rest[:-len(endpoint)].rstrip("/")
                upstream_base = f"{proto}://{host}{base_sub}"
                if not upstream_base.endswith("/v1"):
                    upstream_base = f"{upstream_base}/v1"
                return upstream_base, endpoint
        upstream_base = f"{proto}://{host}"
        if not upstream_base.endswith("/v1"):
            upstream_base = f"{upstream_base}/v1"
        return upstream_base, rest

    return "https://wawazz.xyz/v1", "/" + cleaned


@app.head("/{full_path:path}")
async def handle_head(full_path: str):
    logger.info("HEAD request to /%s", full_path)
    return Response(status_code=200)


@app.get("/health")
async def health():
    return {"status": "ok", "service": "claude-switch-bridge"}


@app.api_route("/{full_path:path}", methods=["GET", "POST", "OPTIONS"])
async def route_request(request: Request, full_path: str):
    if request.method == "OPTIONS":
        return Response(status_code=200)

    upstream_base, endpoint = parse_upstream_target("/" + full_path)
    logger.info("Incoming %s /%s -> Upstream Base: %s, Endpoint: %s", request.method, full_path, upstream_base, endpoint)

    # Healthcheck probe from Claude Code
    if endpoint == "/api/hello" or full_path.endswith("/api/hello"):
        return Response(status_code=200)

    # Auth token from incoming request
    auth_header = request.headers.get("x-api-key") or request.headers.get("authorization", "")
    if auth_header.startswith("Bearer "):
        api_key = auth_header[7:].strip()
    else:
        api_key = auth_header.strip()

    if not api_key:
        logger.warning("No API key provided in request headers")
        return JSONResponse(
            status_code=401,
            content={"type": "error", "error": {"type": "authentication_error", "message": "Missing API key in x-api-key or Authorization header"}}
        )

    # Read Anthropic request body
    try:
        anthropic_body = await request.json()
    except Exception as e:
        logger.error("Failed to parse JSON body: %s", e)
        return JSONResponse(
            status_code=400,
            content={"type": "error", "error": {"type": "invalid_request_error", "message": str(e)}}
        )

    # Resolve target model
    incoming_model = anthropic_body.get("model", "")
    if not incoming_model or incoming_model.startswith("claude-"):
        target_model = DEFAULT_FALLBACK_MODEL
    else:
        target_model = incoming_model

    # Strip [1m] suffix for upstream OpenAI compatibility
    target_model = re.sub(r"(?i)\[1m\]$", "", target_model)

    # Handle /v1/messages/count_tokens endpoint (token preflight)
    if endpoint == "/v1/messages/count_tokens" or full_path.endswith("/v1/messages/count_tokens"):
        try:
            msgs = anthropic_body.get("messages", [])
            tokens = litellm.token_counter(model="gpt-4o", messages=msgs)
            return JSONResponse(content={"input_tokens": tokens})
        except Exception as e:
            logger.warning("Failed to count tokens locally, returning safe estimate: %s", e)
            chars = sum(len(str(m.get("content", ""))) for m in anthropic_body.get("messages", []))
            return JSONResponse(content={"input_tokens": max(1, chars // 4)})

    logger.info("Routing request: Model %s -> upstream %s | Upstream %s", incoming_model, target_model, upstream_base)

    # Translate Anthropic parameters to OpenAI format via comprehensive adapter
    try:
        (
            openai_params,
            tool_name_mapping,
        ) = adapter.translate_completion_input_params_with_tool_mapping(
            anthropic_body,
            custom_llm_provider="openai",
        )
    except Exception as e:
        logger.error("Adapter input translation error: %s", e)
        return JSONResponse(
            status_code=400,
            content={"type": "error", "error": {"type": "invalid_request_error", "message": f"Translation error: {e}"}}
        )

    # Anthropic API defaults to non-streaming when "stream" is omitted (e.g. auto mode classifier).
    is_stream = anthropic_body.get("stream", False)

    out_model = incoming_model if incoming_model else target_model
    max_pre_emit_retries = 2
    backoff_base = 0.25

    try:
        # Agnostic hydration: start from adapter-translated OpenAI request dictionary,
        # then inject custom routing, credentials, and resilience policies.
        kwargs = {
            **openai_params,
            "model": f"openai/{target_model}",
            "api_base": upstream_base,
            "api_key": api_key,
            "stream": is_stream,
            "num_retries": 2,
        }
        if is_stream:
            kwargs["stream_options"] = {"include_usage": True}

        logger.info("Dispatching to litellm.acompletion (stream=%s)...", is_stream)
        if is_stream:
            # Phase 1: PRE_EMIT - transparent retry loop before HTTP headers are sent.
            first_chunk = None
            anthropic_stream = None

            for attempt in range(max_pre_emit_retries + 1):
                try:
                    response = await litellm.acompletion(**kwargs)
                    anthropic_stream = adapter.translate_completion_output_params_streaming(
                        response,
                        model=out_model,
                        tool_name_mapping=tool_name_mapping,
                        is_async=True
                    )
                    # Verify stream is alive by prefetching the initial chunk
                    first_chunk = await async_next_chunk(anthropic_stream)
                    break
                except Exception as e:
                    if attempt < max_pre_emit_retries and is_transient_error(e):
                        delay = backoff_base * (2 ** attempt) + random.uniform(0.05, 0.15)
                        logger.warning(
                            "Transient upstream error in PRE_EMIT attempt %d/%d (%s); retrying in %.2fs...",
                            attempt + 1, max_pre_emit_retries + 1, e, delay
                        )
                        await asyncio.sleep(delay)
                        continue
                    raise

            # Phase 2: POST_EMIT - stream established; yield first chunk then iterate remainder.
            async def stream_generator():
                nonlocal anthropic_stream
                try:
                    if first_chunk:
                        if isinstance(first_chunk, str):
                            yield first_chunk.encode("utf-8")
                        else:
                            yield first_chunk

                    if anthropic_stream:
                        async for chunk in anthropic_stream:
                            if chunk:
                                if isinstance(chunk, str):
                                    yield chunk.encode("utf-8")
                                else:
                                    yield chunk
                except Exception as e:
                    logger.error("Error streaming completion (POST_EMIT mid-stream): %s", e, exc_info=True)
                    # HTTP headers have already been sent; report the error as
                    # an Anthropic SSE event instead of dropping the connection.
                    _, payload = completion_error(e)
                    yield f"event: error\ndata: {json.dumps(payload)}\n\n".encode("utf-8")
                finally:
                    if anthropic_stream and hasattr(anthropic_stream, "aclose"):
                        try:
                            await anthropic_stream.aclose()
                        except Exception:
                            pass

            return StreamingResponse(
                stream_generator(),
                media_type="text/event-stream",
                headers={
                    "Cache-Control": "no-cache",
                    "Connection": "keep-alive",
                    "Content-Type": "text/event-stream",
                }
            )
        else:
            # Non-streaming request (100% PRE_EMIT)
            response = None
            for attempt in range(max_pre_emit_retries + 1):
                try:
                    response = await litellm.acompletion(**kwargs)
                    break
                except Exception as e:
                    if attempt < max_pre_emit_retries and is_transient_error(e):
                        delay = backoff_base * (2 ** attempt) + random.uniform(0.05, 0.15)
                        logger.warning(
                            "Transient upstream error in non-stream attempt %d/%d (%s); retrying in %.2fs...",
                            attempt + 1, max_pre_emit_retries + 1, e, delay
                        )
                        await asyncio.sleep(delay)
                        continue
                    raise

            # Guard against upstream responses missing the usage field
            if not getattr(response, "usage", None):
                try:
                    response.usage = litellm.Usage(prompt_tokens=0, completion_tokens=0, total_tokens=0)
                except Exception:
                    pass

            anthropic_resp = adapter.translate_completion_output_params(
                response,
                tool_name_mapping=tool_name_mapping
            )
            anthropic_resp["model"] = out_model

            # Usage invariant guard: ensure input_tokens and output_tokens always exist
            # to protect client deserialization from TypeErrors.
            if "usage" not in anthropic_resp or not isinstance(anthropic_resp["usage"], dict):
                anthropic_resp["usage"] = {"input_tokens": 0, "output_tokens": 0}
            else:
                anthropic_resp["usage"].setdefault("input_tokens", 0)
                anthropic_resp["usage"].setdefault("output_tokens", 0)

            return JSONResponse(content=anthropic_resp)

    except Exception as e:
        logger.error("Error executing completion: %s", e, exc_info=True)
        status, payload = completion_error(e)
        return JSONResponse(status_code=status, content=payload)


if __name__ == "__main__":
    import sys
    import fcntl

    port = int(os.environ.get("BRIDGE_PORT", "8088"))
    lock_path = os.environ.get("BRIDGE_LOCK_PATH")
    lock_file = None
    if lock_path:
        try:
            os.makedirs(os.path.dirname(lock_path), exist_ok=True)
            lock_file = open(lock_path, "a")
            fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except (IOError, OSError) as e:
            logger.error("Another bridge daemon is holding lock %s: %s", lock_path, e)
            sys.stderr.write(f"ERROR: bridge daemon lock already held ({e})\n")
            sys.exit(48)

    uvicorn.run(app, host="127.0.0.1", port=port, log_level="info")
