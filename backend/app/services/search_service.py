"""Meilisearch integration — indexing and search."""
from __future__ import annotations

import logging
from typing import Any

import meilisearch

from app.config import settings

logger = logging.getLogger(__name__)

_client: meilisearch.Client | None = None


def get_client() -> meilisearch.Client:
    global _client
    if _client is None:
        _client = meilisearch.Client(settings.meili_url, settings.meili_master_key)
    return _client


def setup_indexes() -> None:
    """Create indexes and configure their settings. Called on app startup."""
    try:
        client = get_client()

        # ── Messages ────────────────────────────────────────────
        try:
            client.create_index("messages", {"primaryKey": "id"})
        except Exception:
            pass  # already exists

        idx = client.index("messages")
        idx.update_searchable_attributes(["content", "authorDisplayName"])
        idx.update_filterable_attributes(["channelId", "authorId", "isPinned", "status"])
        idx.update_sortable_attributes(["createdAt"])
        idx.update_ranking_rules([
            "words", "typo", "proximity", "attribute", "sort", "exactness",
        ])

        # ── Channels ────────────────────────────────────────────
        try:
            client.create_index("channels", {"primaryKey": "id"})
        except Exception:
            pass

        client.index("channels").update_searchable_attributes(["name", "description"])
        client.index("channels").update_filterable_attributes(["type", "isArchived"])

        # ── Users ───────────────────────────────────────────────
        try:
            client.create_index("users", {"primaryKey": "id"})
        except Exception:
            pass

        client.index("users").update_searchable_attributes(["username", "displayName"])
        client.index("users").update_filterable_attributes(["role"])

        logger.info("Meilisearch indexes configured")
    except Exception as exc:
        logger.warning("Meilisearch unavailable at startup: %s", exc)


# ── Message indexing ──────────────────────────────────────────

def _msg_doc(msg: Any) -> dict:
    """Convert a MessageOut schema to a Meilisearch document."""
    return {
        "id": msg.id,
        "channelId": msg.channelId,
        "authorId": msg.authorId,
        "authorDisplayName": msg.author.displayName if msg.author else "",
        "content": msg.content,
        "status": msg.status,
        "isPinned": msg.isPinned,
        "createdAt": msg.createdAt,
    }


def index_message(msg: Any) -> None:
    try:
        get_client().index("messages").add_documents([_msg_doc(msg)])
    except Exception as exc:
        logger.debug("Meilisearch index_message failed: %s", exc)


def update_message_index(msg: Any) -> None:
    try:
        get_client().index("messages").update_documents([_msg_doc(msg)])
    except Exception as exc:
        logger.debug("Meilisearch update_message_index failed: %s", exc)


def delete_message_index(message_id: str) -> None:
    try:
        get_client().index("messages").delete_document(message_id)
    except Exception as exc:
        logger.debug("Meilisearch delete_message_index failed: %s", exc)


# ── Channel / User indexing ───────────────────────────────────

def index_channel(ch: Any) -> None:
    try:
        doc = {
            "id": ch.id,
            "name": ch.name,
            "description": ch.description or "",
            "type": ch.type,
            "isArchived": ch.isArchived,
        }
        get_client().index("channels").add_documents([doc])
    except Exception as exc:
        logger.debug("Meilisearch index_channel failed: %s", exc)


def index_user(user: Any) -> None:
    try:
        doc = {
            "id": str(user.id),
            "username": user.username,
            "displayName": user.display_name or user.username,
            "role": user.role.value,
        }
        get_client().index("users").add_documents([doc])
    except Exception as exc:
        logger.debug("Meilisearch index_user failed: %s", exc)


# ── Search ───────────────────────────────────────────────────

def search(
    query: str,
    channel_id: str | None = None,
    limit: int = 20,
) -> dict[str, list]:
    """Run searches across messages, channels, and users."""
    client = get_client()

    def _search_index(index_uid: str, q: str, opts: dict) -> list:
        try:
            return client.index(index_uid).search(q, opts).get("hits", [])
        except Exception as exc:
            logger.debug("Meilisearch search [%s] failed: %s", index_uid, exc)
            return []

    msg_filter: list[str] = ['status = "active"']
    if channel_id:
        msg_filter.append(f'channelId = "{channel_id}"')

    messages = _search_index("messages", query, {
        "limit": limit,
        "filter": " AND ".join(msg_filter),
        "attributesToHighlight": ["content"],
        "highlightPreTag": "<mark>",
        "highlightPostTag": "</mark>",
    })
    channels = _search_index("channels", query, {
        "limit": 5,
        "filter": 'isArchived = false',
    })
    users = _search_index("users", query, {
        "limit": 5,
    })

    return {"messages": messages, "channels": channels, "users": users}


def reindex_all_messages(messages: list[Any]) -> None:
    """Bulk re-index all messages (called from admin endpoint)."""
    try:
        docs = [_msg_doc(m) for m in messages]
        if docs:
            get_client().index("messages").add_documents(docs)
        logger.info("Reindexed %d messages", len(docs))
    except Exception as exc:
        logger.warning("Meilisearch reindex_all failed: %s", exc)
