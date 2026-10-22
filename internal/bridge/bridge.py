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
import sys
import time
import json
import fcntl
import asyncio
import hashlib
import logging
import random
import warnings
from typing import Optional
from contextlib import asynccontextmanager
from fastapi import FastAPI, Request, Response, HTTPException
from fastapi.responses import StreamingResponse, JSONResponse
from starlette.concurrency import run_in_threadpool
import httpx
import uvicorn
import litellm
from litellm.llms.anthropic.pass_through.adapters.transformation import AnthropicAdapter

# Suppress pydantic serialization warnings for custom schema fields
warnings.filterwarnings("ignore", category=UserWarning, module="pydantic")

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

STREAM_ONLY_UPSTREAMS: set[str] = set()

_codex_semaphore: Optional[asyncio.Semaphore] = None


def get_codex_semaphore() -> asyncio.Semaphore:
    """Returns an asyncio.Semaphore to gate concurrent initiations to Codex backend."""
    global _codex_semaphore
    if _codex_semaphore is None:
        _codex_semaphore = asyncio.Semaphore(2)
    return _codex_semaphore

STATE_READY = "READY"
STATE_DRAINING = "DRAINING"

server_state = STATE_READY
active_requests = 0
global_server: Optional[uvicorn.Server] = None
lock_file = None


def get_script_hash() -> str:
    path = __file__
    if path.endswith(".pyc"):
        path = path[:-1]
    try:
        with open(path, "rb") as f:
            return hashlib.sha256(f.read()).hexdigest()
    except Exception:
        return ""


CONTROL_PATHS = {"/health", "/control/drain", "/control/status"}


@app.middleware("http")
async def track_active_requests(request: Request, call_next):
    global active_requests
    path = request.url.path
    is_control = path in CONTROL_PATHS or path.rstrip("/") in CONTROL_PATHS

    if not is_control and server_state == STATE_DRAINING:
        return JSONResponse(
            status_code=503,
            headers={"Connection": "close"},
            content={"type": "error", "error": {"type": "service_unavailable", "message": "Bridge is draining and retiring"}},
        )

    active_requests += 1
    try:
        return await call_next(request)
    finally:
        active_requests -= 1


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


def is_stream_required_error(error: Exception) -> bool:
    """Determine whether an error indicates the upstream group only accepts streaming requests."""
    msg = str(error).lower()
    return (
        "stream_required" in msg
        or "only accepts streaming requests" in msg
        or '"stream": true' in msg
        or "'stream': true" in msg
    )


async def aggregate_stream_to_model_response(stream_response, messages: list) -> litellm.ModelResponse:
    """Aggregate chunks from an upstream streaming response into a single ModelResponse."""
    chunks = []
    max_chunks = 100_000
    if hasattr(stream_response, "__aiter__"):
        async for chunk in stream_response:
            chunks.append(chunk)
            if len(chunks) > max_chunks:
                raise HTTPException(status_code=413, detail="Stream response exceeded aggregation buffer limits")
    elif hasattr(stream_response, "__iter__"):
        for chunk in stream_response:
            chunks.append(chunk)
            if len(chunks) > max_chunks:
                raise HTTPException(status_code=413, detail="Stream response exceeded aggregation buffer limits")
    else:
        return stream_response

    if not chunks:
        return litellm.ModelResponse(
            choices=[{"index": 0, "message": {"role": "assistant", "content": ""}, "finish_reason": "stop"}],
            usage={"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
        )
    return litellm.stream_chunk_builder(chunks, messages=messages)


def completion_error(error: Exception) -> tuple[int, dict, dict[str, str]]:
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

    headers: dict[str, str] = {}
    err_str = str(error)

    if "usage_limit_reached" in err_str:
        resets_in = None
        m_sec = re.search(r'"resets_in_seconds"\s*:\s*(\d+)', err_str)
        if m_sec:
            resets_in = int(m_sec.group(1))

        m_at = re.search(r'"resets_at"\s*:\s*(\d+)', err_str)
        resets_at_str = ""
        resets_at_val = None
        if m_at:
            import datetime
            ts = int(m_at.group(1))
            resets_at_val = ts
            resets_at_str = f" at {datetime.datetime.fromtimestamp(ts).strftime('%H:%M:%S')}"
        elif resets_in is not None:
            resets_at_val = int(time.time() + resets_in)

        # Inject Anthropic unified rate-limit headers so Claude Code natively
        # triggers onQuotaWindowWait (suspends session and resumes when quota recovers).
        headers["anthropic-ratelimit-unified-status"] = "rejected"
        if resets_at_val is not None:
            headers["anthropic-ratelimit-unified-reset"] = str(resets_at_val)

        if resets_in is not None:
            headers["retry-after"] = str(resets_in)
            mins = max(1, round(resets_in / 60))
            friendly_msg = (
                f"ChatGPT Plus Codex usage limit reached (5-hour window). "
                f"Quota resets in {mins} minutes{resets_at_str}."
            )
        else:
            friendly_msg = "ChatGPT Plus Codex usage limit reached (5-hour window)."
    elif "litellm." in err_str:
        friendly_msg = re.sub(r"^(litellm\.\w+:\s*)+", "", err_str).strip()
    else:
        friendly_msg = err_str

    return status, {
        "type": "error",
        "error": {"type": error_type, "message": friendly_msg},
    }, headers


def _atomic_write_json(path: str, data: dict, mode: int = 0o600) -> None:
    dir_name = os.path.dirname(path)
    os.makedirs(dir_name, exist_ok=True)
    tmp_path = f"{path}.tmp.{os.getpid()}"
    try:
        flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC
        fd = os.open(tmp_path, flags, mode)
        with open(fd, "w", encoding="utf-8") as f:
            json.dump(data, f, indent=2)
        os.replace(tmp_path, path)
    finally:
        if os.path.exists(tmp_path):
            try:
                os.remove(tmp_path)
            except Exception:
                pass


def sync_codex_auth() -> None:
    """Bidirectionally synchronize OAuth tokens between ~/.codex/auth.json and LiteLLM's auth file with atomic writes and 0600 permissions."""
    codex_file = os.path.expanduser("~/.codex/auth.json")
    litellm_dir = os.path.expanduser("~/.config/litellm/chatgpt")
    litellm_file = os.path.join(litellm_dir, "auth.json")
    if not os.path.exists(codex_file):
        return
    try:
        with open(codex_file, "r", encoding="utf-8") as f:
            c_data = json.load(f)
        c_tokens = c_data.get("tokens", {}) if isinstance(c_data, dict) else {}

        l_data = {}
        if os.path.exists(litellm_file):
            try:
                with open(litellm_file, "r", encoding="utf-8") as f:
                    l_data = json.load(f)
            except Exception:
                l_data = {}

        c_access = c_tokens.get("access_token")
        l_access = l_data.get("access_token")

        # Case 1: LiteLLM file doesn't exist or codex has a different access token
        if c_access and c_access != l_access:
            litellm_payload = {
                "access_token": c_access,
                "refresh_token": c_tokens.get("refresh_token"),
                "id_token": c_tokens.get("id_token"),
                "account_id": c_tokens.get("account_id"),
            }
            _atomic_write_json(litellm_file, litellm_payload, 0o600)
        # Case 2: LiteLLM refreshed the access token (and codex does not have it yet)
        elif l_access and l_access != c_access and isinstance(c_data, dict) and "tokens" in c_data:
            c_data["tokens"]["access_token"] = l_access
            if l_data.get("refresh_token"):
                c_data["tokens"]["refresh_token"] = l_data["refresh_token"]
            if l_data.get("id_token"):
                c_data["tokens"]["id_token"] = l_data["id_token"]
            if l_data.get("account_id"):
                c_data["tokens"]["account_id"] = l_data["account_id"]
            _atomic_write_json(codex_file, c_data, 0o600)
    except Exception as e:
        logger.warning("Error syncing auth between codex and litellm: %s", e)


def parse_upstream_target(path: str) -> tuple[str, str]:
    """
    Extracts the upstream base URL and the sub-endpoint from the incoming path.
    Supported path patterns:
      /codex/v1/messages                                -> (http://127.0.0.1:8787/v1, /v1/messages)
      /https/<host>/<subpath>/v1/messages/count_tokens -> (https://<host>/<subpath>/v1, /v1/messages/count_tokens)
      /https/<host>/<subpath>/v1/messages              -> (https://<host>/<subpath>/v1, /v1/messages)
      /http/<host>/<subpath>/v1/messages               -> (http://<host>/<subpath>/v1, /v1/messages)
      /proxy/<url>/v1/messages                          -> (<url>, /v1/messages)
    """
    cleaned = path.lstrip("/")
    if cleaned == "codex" or cleaned.startswith("codex/"):
        rest = "/" + cleaned.split("/", 1)[1] if "/" in cleaned else ""
        for endpoint in ["/v1/messages/count_tokens", "/v1/messages", "/api/hello", "/v1/models"]:
            if rest.endswith(endpoint):
                return "http://127.0.0.1:8787/v1", endpoint
        return "http://127.0.0.1:8787/v1", rest

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
    return {
        "status": "ok",
        "service": "claude-switch-bridge",
        "hash": get_script_hash(),
        "state": server_state,
        "active_requests": active_requests,
        "pid": os.getpid(),
        "supports_drain": True,
    }


async def drain_and_shutdown(server: Optional[uvicorn.Server], timeout: float = 60.0):
    await asyncio.sleep(0.05)  # Allow HTTP response to flush over network
    deadline = asyncio.get_event_loop().time() + timeout
    logger.info("Graceful drain started (timeout=%.1fs, active_requests=%d)...", timeout, active_requests)
    while active_requests > 0 and asyncio.get_event_loop().time() < deadline:
        await asyncio.sleep(0.1)
    logger.info("Graceful drain complete (active_requests=%d). Exiting server.", active_requests)
    if server:
        server.should_exit = True
    else:
        sys.exit(0)


@app.post("/control/drain")
async def control_drain():
    global server_state, lock_file
    logger.info("Control drain requested. Current state=%s, active_requests=%d", server_state, active_requests)
    server_state = STATE_DRAINING

    # 1. Release daemon lock so new daemon can acquire it immediately
    if lock_file:
        try:
            fcntl.flock(lock_file, fcntl.LOCK_UN)
            lock_file.close()
            lock_file = None
            logger.info("Released daemon file lock")
        except Exception as e:
            logger.warning("Error releasing daemon lock: %s", e)

    # 2. Close listening socket so port is immediately freed for new daemon
    if global_server and hasattr(global_server, "servers"):
        for s in global_server.servers:
            try:
                s.close()
            except Exception as e:
                logger.warning("Error closing listening socket: %s", e)
        logger.info("Closed listener sockets on port")

    # 3. Schedule background drain task
    asyncio.create_task(drain_and_shutdown(global_server, timeout=60.0))

    return {
        "status": "draining",
        "hash": get_script_hash(),
        "active_requests": active_requests,
        "pid": os.getpid(),
    }


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

    # Identify if upstream is direct ChatGPT subscription backend (bypassing local proxy)
    is_codex = "chatgpt.com" in upstream_base

    # Resolve target model across both official Codex and third-party relay
    incoming_model = anthropic_body.get("model", "")
    if not incoming_model or incoming_model.startswith("claude-"):
        if "haiku" in incoming_model:
            target_model = "gpt-6-luna"
        elif "opus" in incoming_model:
            target_model = "gpt-6.1-sol"
        elif "sonnet" in incoming_model:
            target_model = "gpt-6.1-sol"
        elif "fable" in incoming_model:
            target_model = "gpt-6-astra"
        else:
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

    logger.info("Routing request: Model %s -> upstream %s | Upstream %s (is_codex=%s)", incoming_model, target_model, upstream_base, is_codex)

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

    is_codex_direct = "chatgpt.com" in upstream_base
    is_codex_proxy = ":8787" in upstream_base or "codex" in upstream_base
    if is_codex_direct or is_codex_proxy:
        sync_codex_auth()

    if is_codex_direct:
        model_name = f"chatgpt/{target_model}"
        # Normalize and consolidate system messages for ChatGPT Codex Responses API.
        # Codex rejects {"role": "system"} inside input items; all system text must be
        # a single leading string so LiteLLM maps it cleanly to top-level instructions.
        if "messages" in openai_params:
            system_texts = []
            non_system_messages = []
            for msg in openai_params["messages"]:
                if msg.get("role") == "system":
                    content = msg.get("content")
                    if isinstance(content, list):
                        text = "\n\n".join(
                            part.get("text", "")
                            for part in content
                            if isinstance(part, dict) and part.get("type") == "text"
                        )
                    else:
                        text = str(content or "")
                    if text:
                        system_texts.append(text)
                else:
                    non_system_messages.append(msg)

            if system_texts:
                merged_system = "\n\n".join(system_texts)
                openai_params["messages"] = [{"role": "system", "content": merged_system}] + non_system_messages
            else:
                openai_params["messages"] = non_system_messages
    else:
        model_name = f"openai/{target_model}"

    try:
        # Agnostic hydration: start from adapter-translated OpenAI request dictionary,
        # then inject custom routing, credentials, and resilience policies.
        kwargs = {
            **openai_params,
            "model": model_name,
            "stream": is_stream,
            "num_retries": 2,
        }
        if not is_codex_direct:
            kwargs["api_base"] = upstream_base
            kwargs["api_key"] = api_key
        if is_stream:
            kwargs["stream_options"] = {"include_usage": True}

        session_id = (
            request.headers.get("x-claude-code-session-id")
            or request.headers.get("x-session-id")
            or request.headers.get("session-id")
            or os.environ.get("CODEX_COMPANION_SESSION_ID")
        )
        if session_id:
            kwargs.setdefault("extra_headers", {})["x-session-id"] = session_id
            kwargs["extra_headers"]["x-claude-code-session-id"] = session_id

        logger.info("Dispatching to litellm.acompletion (stream=%s)...", is_stream)
        use_codex_semaphore = is_codex_direct or is_codex_proxy
        codex_sem = get_codex_semaphore() if use_codex_semaphore else None

        if is_stream:
            # Phase 1: PRE_EMIT - transparent retry loop before HTTP headers are sent.
            first_chunk = None
            anthropic_stream = None

            for attempt in range(max_pre_emit_retries + 1):
                try:
                    if codex_sem:
                        async with codex_sem:
                            response = await litellm.acompletion(**kwargs)
                            anthropic_stream = adapter.translate_completion_output_params_streaming(
                                response,
                                model=out_model,
                                tool_name_mapping=tool_name_mapping,
                                is_async=True
                            )
                            # Verify stream is alive by prefetching the initial chunk
                            first_chunk = await async_next_chunk(anthropic_stream)
                    else:
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
                    _, payload, _ = completion_error(e)
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
            # Adaptive stream check: if upstream is known to require streaming, fetch via stream aggregation
            if upstream_base in STREAM_ONLY_UPSTREAMS:
                kwargs["stream"] = True
                kwargs["stream_options"] = {"include_usage": True}
                logger.info("Upstream %s is known stream-only; fetching via stream aggregation...", upstream_base)

            response = None
            msgs = anthropic_body.get("messages", [])
            for attempt in range(max_pre_emit_retries + 1):
                try:
                    if codex_sem:
                        async with codex_sem:
                            response = await litellm.acompletion(**kwargs)
                            if kwargs.get("stream"):
                                response = await aggregate_stream_to_model_response(response, messages=msgs)
                    else:
                        response = await litellm.acompletion(**kwargs)
                        if kwargs.get("stream"):
                            response = await aggregate_stream_to_model_response(response, messages=msgs)
                    break
                except Exception as e:
                    if not kwargs.get("stream") and is_stream_required_error(e):
                        logger.info(
                            "Upstream %s rejected non-streaming request with stream_required; "
                            "marking upstream as stream-only and falling back to stream aggregation...",
                            upstream_base
                        )
                        STREAM_ONLY_UPSTREAMS.add(upstream_base)
                        kwargs["stream"] = True
                        kwargs["stream_options"] = {"include_usage": True}
                        try:
                            stream_resp = await litellm.acompletion(**kwargs)
                            response = await aggregate_stream_to_model_response(stream_resp, messages=msgs)
                            break
                        except Exception as stream_err:
                            e = stream_err

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
            if not getattr(response, "usage", None) or getattr(response.usage, "total_tokens", 0) == 0:
                try:
                    messages = openai_params.get("messages", [])
                    calc_in = litellm.token_counter(model=model_name, messages=messages) if messages else 0
                    resp_text = ""
                    if getattr(response, "choices", None) and len(response.choices) > 0:
                        resp_text = getattr(response.choices[0].message, "content", "") or ""
                    calc_out = litellm.token_counter(model=model_name, text=resp_text) if resp_text else 0
                    response.usage = litellm.Usage(
                        prompt_tokens=calc_in,
                        completion_tokens=calc_out,
                        total_tokens=calc_in + calc_out
                    )
                except Exception:
                    if not getattr(response, "usage", None):
                        response.usage = litellm.Usage(prompt_tokens=0, completion_tokens=0, total_tokens=0)

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
        status, payload, err_headers = completion_error(e)
        return JSONResponse(status_code=status, content=payload, headers=err_headers or None)

async def run_server(port: int):
    global global_server
    config = uvicorn.Config(app, host="127.0.0.1", port=port, log_level="info")
    global_server = uvicorn.Server(config)
    await global_server.serve()


if __name__ == "__main__":
    port = int(os.environ.get("BRIDGE_PORT", "8088"))
    lock_path = os.environ.get("BRIDGE_LOCK_PATH")
    if lock_path:
        try:
            os.makedirs(os.path.dirname(lock_path), exist_ok=True)
            lock_file = open(lock_path, "a")
            fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except (IOError, OSError) as e:
            logger.error("Another bridge daemon is holding lock %s: %s", lock_path, e)
            sys.stderr.write(f"ERROR: bridge daemon lock already held ({e})\n")
            sys.exit(48)

    asyncio.run(run_server(port))
