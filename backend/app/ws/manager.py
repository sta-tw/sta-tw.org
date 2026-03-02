"""In-process WebSocket connection manager.
Each connection is global; clients subscribe to specific channels via messages.
Phase 5+: replace broadcast with Redis pub/sub for multi-process scaling.
"""
from typing import Any

from fastapi import WebSocket


class ConnectionManager:
    def __init__(self) -> None:
        # channel_id -> set of WebSocket connections subscribed to it
        self._channels: dict[str, set[WebSocket]] = {}
        # id(ws) -> set of channel_ids (for fast cleanup on disconnect)
        self._ws_channels: dict[int, set[str]] = {}

    async def connect(self, ws: WebSocket) -> None:
        """Accept a new WebSocket connection (not yet subscribed to any channel)."""
        await ws.accept()
        self._ws_channels[id(ws)] = set()

    def subscribe(self, ws: WebSocket, channel_id: str) -> None:
        """Subscribe ws to broadcasts for channel_id."""
        self._channels.setdefault(channel_id, set()).add(ws)
        if id(ws) in self._ws_channels:
            self._ws_channels[id(ws)].add(channel_id)

    def unsubscribe(self, ws: WebSocket, channel_id: str) -> None:
        """Unsubscribe ws from channel_id."""
        if channel_id in self._channels:
            self._channels[channel_id].discard(ws)
        if id(ws) in self._ws_channels:
            self._ws_channels[id(ws)].discard(channel_id)

    def disconnect(self, ws: WebSocket) -> None:
        """Remove ws from all channel subscriptions."""
        ws_id = id(ws)
        subscribed = self._ws_channels.pop(ws_id, set())
        for channel_id in subscribed:
            self._channels.get(channel_id, set()).discard(ws)

    async def broadcast(
        self, channel_id: str, event: dict[str, Any], exclude_ws: WebSocket | None = None
    ) -> None:
        for ws in list(self._channels.get(channel_id, set())):
            if ws is exclude_ws:
                continue
            try:
                await ws.send_json(event)
            except Exception:
                pass  # stale connection; cleaned up on disconnect


manager = ConnectionManager()
