"""Shared process runtime helpers: JSON logging and cooperative shutdown.

The Go services log JSON via slog; these helpers make the Python workers emit
the same shape (``time``/``level``/``msg`` plus any ``extra`` fields) and stop
cleanly on SIGTERM/SIGINT so container stops and ``docker compose down`` drain
at the next loop boundary instead of being killed.
"""

from __future__ import annotations

import json
import logging
import signal
import threading
import time
from typing import Any

_RESERVED = frozenset(
    (
        "name", "msg", "args", "levelname", "levelno", "pathname", "filename",
        "module", "exc_info", "exc_text", "stack_info", "lineno", "funcName",
        "created", "msecs", "relativeCreated", "thread", "threadName",
        "processName", "process", "taskName",
    )
)


class _JSONFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, Any] = {
            "time": time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(record.created))
            + f".{int(record.msecs):03d}Z",
            "level": record.levelname,
            "logger": record.name,
            "msg": record.getMessage(),
        }
        for key, value in record.__dict__.items():
            if key not in _RESERVED and not key.startswith("_"):
                payload[key] = value
        if record.exc_info:
            payload["error"] = self.formatException(record.exc_info)
        return json.dumps(payload, ensure_ascii=False, default=str)


def configure_logging() -> None:
    """Install the JSON formatter on the root logger (idempotent)."""
    handler = logging.StreamHandler()
    handler.setFormatter(_JSONFormatter())
    root = logging.getLogger()
    root.handlers[:] = [handler]
    root.setLevel(logging.INFO)


class Shutdown:
    """A threading.Event fronted by SIGTERM/SIGINT handlers.

    Use ``wait(seconds)`` in place of ``time.sleep`` so an idle poll returns
    immediately on shutdown, and ``is_set()`` as the loop condition.
    """

    def __init__(self) -> None:
        self._event = threading.Event()
        self._log = logging.getLogger("sta-worker")

    def _handle(self, signum: int, _frame: Any) -> None:
        if not self._event.is_set():
            self._log.info("shutdown signal received", extra={"signal": signal.Signals(signum).name})
        self._event.set()

    def install(self) -> "Shutdown":
        for sig in (signal.SIGTERM, signal.SIGINT):
            try:
                signal.signal(sig, self._handle)
            except ValueError:
                # Not on the main thread; the caller handles shutdown differently.
                pass
        return self

    def is_set(self) -> bool:
        return self._event.is_set()

    def wait(self, seconds: float) -> bool:
        return self._event.wait(seconds)


def install_shutdown() -> Shutdown:
    return Shutdown().install()
