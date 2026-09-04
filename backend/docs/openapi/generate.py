#!/usr/bin/env python3
"""Generate docs/openapi.json from the route literals in internal/**/*.go.

Every route contributes its method, a derived summary, a tag, path params and a
method-aware security block, all machine-accurate. On top of that:

  * keyset list routes (KEYSET) declare limit + cursor and a
    {data: [<item>], next_cursor} response;
  * QUERY adds documented query params to the filter-heavy routes;
  * SCHEMAS + ROUTES attach concrete request/response schemas to every JSON
    endpoint, hand-derived from the handler `map[string]any` / struct shapes.

Schemas are hand-maintained here, so a struct change in Go needs a matching edit
below. Run `python3 docs/openapi/generate.py` after touching a route or a
response shape; CI diffs the output.
"""

from __future__ import annotations

import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
ROUTE_RE = re.compile(r'"(GET|POST|PUT|PATCH|DELETE)\s+(/[^"]*)"')

S = lambda **props: {"type": "object", "properties": props}  # noqa: E731
ARR = lambda ref: {"type": "array", "items": ref}  # noqa: E731
REF = lambda name: {"$ref": f"#/components/schemas/{name}"}  # noqa: E731
STR = {"type": "string"}
INT = {"type": "integer"}
BOOL = {"type": "boolean"}
NUM = {"type": "number"}
DT = {"type": "string", "format": "date-time"}
UUID = {"type": "string", "format": "uuid"}


def data(schema):
    return {"type": "object", "required": ["data"], "properties": {"data": schema}}


def page(item):
    return {
        "type": "object",
        "required": ["data", "next_cursor"],
        "properties": {"data": ARR(item), "next_cursor": {"type": "string", "description": '"" = no more rows'}},
    }


# --------------------------------------------------------------------------- #
#  Component schemas (kept in sync with internal/**/model.go by hand)
# --------------------------------------------------------------------------- #

SCHEMAS: dict[str, dict] = {
    "Error": {
        "type": "object", "required": ["error"],
        "properties": {"error": S(
            code={"type": "string", "description": "stable snake_case discriminator"},
            message={"type": "string", "description": "human text, not stable, mixed en/zh"},
        )},
    },
    "Account": {
        "type": "object", "required": ["id", "username", "identity_status", "account_status", "email_verified"],
        "properties": {"id": UUID, "username": STR,
                       "identity_status": {"type": "string", "enum": ["temporary", "student", "senior"]},
                       "account_status": {"type": "string", "enum": ["active", "suspended", "deleted"]},
                       "email_verified": BOOL},
    },
    "SessionSummary": S(id=UUID, created_at=DT, last_seen_at=DT, expires_at=DT,
                        current={"type": "boolean", "description": "the session making the request"}),
    "ReactionTally": {"type": "object", "required": ["emoji", "count", "mine"],
                      "properties": {"emoji": STR, "count": INT, "mine": BOOL}},
    "ChatChannel": {"type": "object", "required": ["key", "display_name", "kind", "is_default"],
                    "properties": {"key": STR, "display_name": STR,
                                   "kind": {"type": "string", "enum": ["standard", "announcement"]},
                                   "topic": STR, "is_default": BOOL}},
    "ChatMessage": {
        "type": "object", "required": ["id", "body", "source_platform", "status", "created_at"],
        "properties": {"id": UUID, "body": STR,
                       "source_platform": {"type": "string", "enum": ["website", "discord", "telegram"]},
                       "status": {"type": "string", "enum": ["active", "edited", "deleted"]},
                       "created_at": DT, "edited_at": DT, "channel_key": STR,
                       "parent_id": UUID, "forwarded_from_id": UUID, "pinned_at": DT,
                       "reply_count": INT, "reactions": ARR(REF("ReactionTally"))},
    },
    "Notification": {"type": "object", "required": ["id", "kind", "title", "body", "created_at"],
                     "properties": {"id": UUID, "kind": STR, "title": STR, "body": STR,
                                    "read_at": DT, "created_at": DT}},
    "ProfileLink": {"type": "object", "required": ["label", "url"],
                    "properties": {"label": STR, "url": {"type": "string", "format": "uri"}}},
    "Profile": {"type": "object", "required": ["account_id", "username", "identity_status", "links", "has_avatar"],
                "properties": {"account_id": UUID, "username": STR, "identity_status": STR,
                               "display_name": STR, "bio": STR, "links": ARR(REF("ProfileLink")),
                               "has_avatar": BOOL, "avatar_updated_at": DT, "updated_at": DT}},
    "AdminUser": {"type": "object",
                  "required": ["id", "username", "identity_status", "account_status", "email_verified", "is_admin", "created_at"],
                  "properties": {"id": UUID, "username": STR, "identity_status": STR, "account_status": STR,
                                 "email_verified": BOOL, "is_admin": BOOL, "last_login_at": DT,
                                 "suspended_at": DT, "suspension_reason": STR, "created_at": DT}},
    "AuditRow": S(id={"type": "integer", "format": "int64"}, actor_account_id=UUID, action=STR,
                  entity_type=STR, entity_key=STR, before_data={"type": "object"}, after_data={"type": "object"},
                  reason=STR, request_id=STR, created_at=DT),
    "SignedURL": {"type": "object", "required": ["url", "expires_in"],
                  "properties": {"url": {"type": "string", "format": "uri"}, "expires_in": {"type": "integer", "description": "seconds"}}},
    "Health": S(status=STR, checks={"type": "object", "additionalProperties": {"type": "object"}}),

    # applications ---------------------------------------------------------
    "Application": S(id=UUID, program_identifier=STR, school_code=STR, school_name=STR, program_code=STR,
                     program_name=STR, academic_year=INT, status=STR, locked_at=DT),
    "ServiceTicket": S(id=UUID, program_identifier=STR, reason=STR, status=STR, created_at=DT),

    # results -------------------------------------------------------------
    "ResultReport": S(application_id=UUID, result_status=STR, official_rank=INT, quota=INT,
                      front_candidate_count=INT, front_response_count=INT, position_after_declines=INT,
                      reference_probability=NUM, current_willingness=INT, candidate_number_last4=STR,
                      candidate_number_set=BOOL),
    "ResultInquiry": S(id=UUID, round=STR, response_deadline=DT, created_at=DT, responded=BOOL,
                       current_willingness=INT),
    "WillingnessResponse": S(response_id={"type": "integer", "format": "int64"}, academic_year=INT,
                             school_code=STR, program_code=STR, result_status=STR, admission_rank=INT,
                             willingness=INT),
    "AdminResultBatch": S(id=UUID, academic_year=INT, school_code=STR, source_url=STR, source_sha256=STR,
                          status=STR, imported_at=DT, reviewed_at=DT, result_count=INT, matched_count=INT,
                          inquiry_count=INT),

    # verification ------------------------------------------------------
    "VerificationRequest": S(id=UUID, academic_year=INT, school_code=STR, program_code=STR, method=STR,
                             status=STR, document_count=INT, created_at=DT, reviewed_at=DT),
    "VerificationDocument": S(id=UUID, request_id=UUID, original_file_name=STR, mime_type=STR,
                              file_size_bytes={"type": "integer", "format": "int64"}, sha256=STR, status=STR,
                              rejection_reason=STR, reviewed_at=DT, created_at=DT),
    "VerificationDomain": S(id=UUID, school_code=STR, domain=STR, is_active=BOOL, created_at=DT),
    "Verification": S(id=UUID, academic_year=INT, school_code=STR, program_code=STR, method=STR, status=STR,
                      verified_at=DT, expires_at=DT),

    # support ---------------------------------------------------------
    "SupportTicket": S(id=UUID, ticket_number=STR, category=STR, subject=STR, status=STR, assigned_to=UUID,
                       discord_sync_status=STR, created_at=DT, updated_at=DT, closed_at=DT,
                       requester=S(account_id=UUID, username=STR, email=STR)),
    "SupportMessage": S(id=UUID, author_type=STR, source_platform=STR, body=STR, created_at=DT, edited_at=DT,
                        status=STR, attachments=ARR(REF("SupportAttachment"))),
    "SupportAttachment": S(id=UUID, ticket_id=UUID, message_id=UUID, original_name=STR, mime_type=STR,
                           file_size_bytes={"type": "integer", "format": "int64"}, sha256=STR, created_at=DT),
    "SupportTicketDetail": {"type": "object", "properties": {"ticket": REF("SupportTicket"),
                                                             "messages": ARR(REF("SupportMessage"))}},

    # schools / admissions -----------------------------------------------
    "School": S(school_code=STR, school_name=STR, institution_type=STR, is_active=BOOL, created_at=DT, updated_at=DT),
    "SchoolAuditEvent": S(id={"type": "integer", "format": "int64"}, action=STR, entity_key=STR,
                          before_data={"type": "object"}, after_data={"type": "object"}, reason=STR, created_at=DT),
    "ExamItem": S(name=STR, sort_order=INT, weight_percent=NUM, multiplier=NUM, description=STR, source_page=STR),
    "AdmissionProgram": S(academic_year=INT, program_identifier=STR, school_code=STR, school_name=STR,
                          program_code=STR, admission_program_name=STR, admission_quota=INT,
                          willingness_values=ARR(INT), exam_items=ARR(REF("ExamItem")),
                          brochure_is_tentative=BOOL, brochure_announcement_date=STR, brochure_scheduled_date=STR,
                          registration_start_date=STR, registration_end_date=STR, exam_start_date=STR,
                          exam_end_date=STR, result_date=STR, consultation_phone=STR, brochure_url=STR,
                          special_talent_target=STR, notes=STR, source_locator=STR, review_status=STR,
                          created_at=DT, updated_at=DT),
    "BrochureDocument": S(academic_year=INT, school_code=STR, original_file_name=STR, mime_type=STR,
                          file_size_bytes={"type": "integer", "format": "int64"}, sha256=STR, source_url=STR,
                          review_status=STR, published_at=DT, created_at=DT),
    "ProgramAuditEvent": S(id={"type": "integer", "format": "int64"}, action=STR, entity_key=STR,
                           before_data={"type": "object"}, after_data={"type": "object"}, reason=STR, created_at=DT),

    # portfolio ----------------------------------------------------------
    "PortfolioProject": S(id=UUID, application_id=UUID, title=STR, created_at=DT, updated_at=DT),
    "PortfolioFile": S(id=UUID, project_id=UUID, version_number=INT, original_file_name=STR, mime_type=STR,
                       file_size_bytes={"type": "integer", "format": "int64"}, sha256_hex=STR, status=STR,
                       rejection_reason=STR, created_at=DT, updated_at=DT),
    "PortfolioFileEvent": S(id={"type": "integer", "format": "int64"}, action=STR, from_status=STR,
                            to_status=STR, reason=STR, created_at=DT),

    # ingestion --------------------------------------------------------
    "IngestionRun": S(id=UUID, ingestion_job_id=UUID, academic_year=INT, school_code=STR, source_sha256=STR,
                      processor_version=STR, status=STR, raw_extraction={"type": "object"}, error_code=STR,
                      error_message=STR, reviewed_by=UUID, reviewed_at=DT, created_at=DT, updated_at=DT,
                      candidates=ARR(REF("IngestionCandidate"))),
    "IngestionCandidate": S(id=UUID, run_id=UUID, program_code=STR, extracted_data={"type": "object"},
                            source_page=INT, confidence=NUM, review_status=STR, reviewed_by=UUID,
                            reviewed_at=DT, created_at=DT, updated_at=DT),
    "IngestionJobStatus": S(job_id=UUID, job_type=STR, source_type=STR, academic_year=INT, school_code=STR,
                            status=STR, attempt_count=INT, last_error_code=STR, last_error_message=STR,
                            created_at=DT, updated_at=DT),

    # admission sources ------------------------------------------------
    "SourceEvidence": S(url=STR, page_or_locator=STR, text=STR),
    "AdmissionSource": S(id=UUID, school_code=STR, academic_year=INT, source_url=STR, normalized_url=STR,
                         hostname=STR, source_type=STR, status=STR, decision_mode=STR,
                         affiliation_confidence=STR, discovery_method=STR, evidence=ARR(REF("SourceEvidence")),
                         first_seen_at=DT, last_seen_at=DT, last_crawled_at=DT, last_discovery_at=DT,
                         discovery_needed=BOOL, discovery_reason=STR, rejected_reason=STR, manual_note=STR,
                         created_at=DT, updated_at=DT),

    # forum / experiences -------------------------------------------
    "ForumSpace": S(id=UUID, space_type={"type": "string", "enum": ["global", "annual", "school_program"]},
                    display_name=STR, academic_year=INT, school_code=STR, program_code=STR, joined=BOOL),
    "ForumThread": S(id=UUID, space_id=UUID, title=STR, created_at=DT, updated_at=DT),
    "ForumPost": S(id=UUID, thread_id=UUID, body=STR, quoted_experience_id=UUID, created_at=DT,
                   reactions=ARR(REF("ReactionTally"))),
    "Experience": S(id=UUID, author_type=STR, admission_outcome=STR,
                    visibility={"type": "string", "enum": ["hidden", "published", "unpublished"]},
                    current_revision_id=UUID, title=STR, body=STR, revision_number=INT,
                    created_at=DT, updated_at=DT, reactions=ARR(REF("ReactionTally"))),

    # brochure discovery -------------------------------------------
    "DiscoveryCycle": S(academic_year=INT, status=STR, created_by=UUID, started_by=UUID, closed_by=UUID,
                        started_at=DT, closed_at=DT, created_at=DT, updated_at=DT),
    "DiscoveryTask": S(academic_year=INT, school_code=STR, school_name=STR, status=STR, completion_method=STR,
                       attempt_count=INT, candidate_source_url=STR, candidate_document_url=STR, candidate_sha256=STR,
                       candidate_confidence=NUM, candidate_evidence={"type": "object"}, last_error_code=STR,
                       last_error_message=STR, last_searched_at=DT, next_search_at=DT, completed_at=DT,
                       completed_by=UUID, created_at=DT, updated_at=DT),
    "DiscoveryEvent": S(id={"type": "integer", "format": "int64"}, academic_year=INT, school_code=STR, action=STR,
                        from_status=STR, to_status=STR, actor_account_id=UUID, details={"type": "object"}, created_at=DT),

    # extraction jobs (worker/agent) -------------------------------
    "ExtractionJob": S(job_id=UUID, job_type=STR, source_type=STR, academic_year=INT, school_code=STR,
                       status=STR, payload={"type": "object"}, traceparent=STR),
    "ExtractionClaim": S(job=REF("ExtractionJob"), download_url={"type": "string", "format": "uri"},
                         expires_in=INT),

    # stats -----------------------------------------------------
    "OutboxHealth": S(pending={"type": "integer", "format": "int64"},
                      failed={"type": "integer", "format": "int64"},
                      abandoned={"type": "integer", "format": "int64"}),
    "StatsSnapshot": S(
        generated_at=DT,
        accounts=S(total=INT, active=INT, suspended=INT, deleted=INT, students=INT, seniors=INT, verified=INT),
        applications=S(total=INT, draft=INT, confirmed=INT, withdrawn=INT, archived=INT),
        experiences=S(total=INT, published=INT, hidden=INT, unpublished=INT),
        forum=S(spaces=INT, threads=INT, posts=INT),
        chat=S(lounge_messages=INT),
        support_tickets=S(total=INT, open=INT, closed=INT),
        verification_requests=S(pending=INT, approved=INT, rejected=INT),
        result_batches=S(total=INT, pending_review=INT, published=INT),
        audit_log=S(total=INT),
        outbox=S(email=REF("OutboxHealth"), chat_sync=REF("OutboxHealth"),
                 support_discord=REF("OutboxHealth"), willingness_notifications=REF("OutboxHealth")),
    ),

    # telegram cross-check --------------------------------------------
    "CrossCheckDashboard": S(telegram_user_id={"type": "integer", "format": "int64"},
                             notifications_enabled=BOOL,
                             applications=ARR(S(application_id=UUID, program_identifier=STR, academic_year=INT,
                                                school_code=STR, school_name=STR, program_code=STR, program_name=STR,
                                                result_status=STR, official_rank=INT, quota=INT))),
    "CrossCheckAdminStatus": S(participant_count=INT, started_count=INT, notifications_enabled_count=INT,
                               outbox_by_status={"type": "object", "additionalProperties": INT}),
}


# --------------------------------------------------------------------------- #
#  Per-route request / response bindings
# --------------------------------------------------------------------------- #

_APPROVE = S(approved=BOOL, reason=STR)
_REASON = S(reason=STR)

# Keyset list routes -> item schema ($ref name, or None for a generic object).
KEYSET: dict[tuple[str, str], str | None] = {
    ("get", "/api/v1/chat/lounge/messages"): "ChatMessage",
    ("get", "/api/v1/chat/channels/{channelKey}/messages"): "ChatMessage",
    ("get", "/api/v1/chat/messages/{messageID}/replies"): "ChatMessage",
    ("get", "/api/v1/notifications"): "Notification",
    ("get", "/api/v1/experiences"): "Experience",
    ("get", "/api/v1/forum/spaces/{spaceID}/threads"): "ForumThread",
    ("get", "/api/v1/forum/threads/{threadID}/posts"): "ForumPost",
    ("get", "/api/v1/admin/users"): "AdminUser",
    ("get", "/api/v1/admin/audit-log"): "AuditRow",
}

_LIMIT = {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 100, "default": 50}}
_CURSOR = {"name": "cursor", "in": "query", "schema": {"type": "string"}, "description": "opaque; from a prior next_cursor"}
_OFFSET = {"name": "offset", "in": "query", "schema": {"type": "integer", "minimum": 0, "maximum": 10000}}


def _q(name, typ="string", **extra):
    p = {"name": name, "in": "query", "schema": {"type": typ}}
    p.update(extra)
    return p


QUERY: dict[tuple[str, str], list[dict]] = {
    ("get", "/api/v1/search"): [_q("q", description="search term"),
                                _q("types", description="comma list: schools,programs,experiences")],
    ("get", "/api/v1/schools"): [_q("q", description="name/code prefix")],
    ("get", "/api/v1/admissions/programs"): [_q("academic_year", "integer"), _q("school_code"), _q("q")],
    ("get", "/api/v1/admissions/schools"): [_q("academic_year", "integer")],
    ("get", "/api/v1/events"): [_q("channel", description="comma list of chat channel keys (default lounge, max 10)")],
    ("get", "/api/v1/admin/users"): [_q("status", description="active|suspended|deleted"),
                                     _q("identity", description="temporary|student|senior"),
                                     _q("role", description="admin"), _q("q", description="username prefix")],
    ("get", "/api/v1/admin/audit-log"): [_q("entity_type"), _q("entity_key"), _q("action"),
                                         _q("actor", description="actor account UUID"),
                                         _q("since", description="RFC3339"), _q("until", description="RFC3339")],
    ("get", "/api/v1/admin/support/tickets"): [_LIMIT, _OFFSET, _q("status"), _q("category"), _q("assigned_to")],
    ("get", "/api/v1/admin/admissions/programs"): [_LIMIT, _OFFSET, _q("academic_year", "integer"),
                                                   _q("school_code"), _q("review_status")],
    ("get", "/api/v1/admin/admission-sources"): [_LIMIT, _OFFSET, _q("academic_year", "integer"),
                                                 _q("school_code"), _q("status")],
    ("get", "/api/v1/admin/ingestion/brochure-runs"): [_LIMIT, _OFFSET, _q("status"), _q("academic_year", "integer")],
    ("get", "/api/v1/admin/portfolio/files"): [_LIMIT, _OFFSET, _q("status")],
    ("get", "/api/v1/admin/results/batches"): [_LIMIT, _OFFSET, _q("status"), _q("academic_year", "integer")],
}

# method,path -> {summary?, request(schema)?, requestContent?, response(schema)?, status?}
# portfolio download echoes just {url, expires_in}; the others wrap the resource:
# {data: <resource>, url, expires_in}.
PORTFOLIO_DL = REF("SignedURL")


def download_of(resource_ref: dict) -> dict:
    return {"type": "object", "properties": {
        "data": resource_ref, "url": {"type": "string", "format": "uri"},
        "expires_in": {"type": "integer", "description": "seconds"}}}

ROUTES: dict[tuple[str, str], dict] = {
    # ---- auth ---------------------------------------------------------
    ("post", "/api/v1/auth/register"): {
        "summary": "Register a native account",
        "request": S(username=STR, email={"type": "string", "format": "email"},
                     password={"type": "string", "minLength": 12}),
        "response": S(account=REF("Account"), email_verification_queued=BOOL), "status": "201"},
    ("post", "/api/v1/auth/login"): {
        "summary": "Log in with username + password (username only, not email)",
        "request": S(username=STR, password=STR), "response": S(account=REF("Account"), expires_at=DT)},
    ("post", "/api/v1/auth/logout"): {"status": "204"},
    ("get", "/api/v1/auth/me"): {"response": data(REF("Account"))},
    ("get", "/api/v1/auth/sessions"): {"response": data(ARR(REF("SessionSummary")))},
    ("delete", "/api/v1/auth/sessions"): {"summary": "Revoke every other session",
                                          "response": S(revoked=INT)},
    ("delete", "/api/v1/auth/sessions/{sessionID}"): {"summary": "Revoke one other session", "status": "204"},
    ("post", "/api/v1/auth/password-reset/request"): {
        "summary": "Request a password-reset email (always 202, no account enumeration)",
        "request": S(email={"type": "string", "format": "email"}),
        "response": S(status={"type": "string", "enum": ["accepted"]}), "status": "202"},
    ("post", "/api/v1/auth/password-reset/confirm"): {
        "summary": "Set a new password using the emailed token",
        "request": S(token=STR, new_password={"type": "string", "minLength": 12}), "status": "204"},
    ("post", "/api/v1/auth/password/change"): {
        "summary": "Change password for the logged-in account (revokes other sessions)",
        "request": S(current_password=STR, new_password={"type": "string", "minLength": 12}), "status": "204"},
    ("post", "/api/v1/auth/email-verification/confirm"): {"request": S(token=STR), "status": "204"},
    ("post", "/api/v1/auth/email-verification/resend"): {"response": S(email_verification_queued=BOOL, expires_at=DT),
                                                         "status": "202"},
    ("get", "/api/v1/auth/admin-mfa/status"): {"response": S(enabled=BOOL, required=BOOL)},
    ("post", "/api/v1/auth/admin-mfa/setup"): {
        "summary": "Begin TOTP enrolment; returns the shared secret + otpauth URL",
        "response": S(secret=STR, otpauth_url=STR, expires_at=DT)},
    ("post", "/api/v1/auth/admin-mfa/enable"): {"request": S(code=STR), "response": S(enabled=BOOL)},
    ("post", "/api/v1/auth/admin-mfa/verify"): {
        "summary": "Verify a TOTP code and open the admin MFA grant window",
        "request": S(code=STR), "response": S(verified=BOOL, expires_at=DT, grant_seconds=INT)},
    ("post", "/api/v1/auth/admin-mfa/disable"): {"request": S(code=STR), "response": S(enabled=BOOL)},
    ("get", "/api/v1/auth/oauth/{provider}/start"): {"summary": "Get the provider authorization URL",
                                                     "response": S(authorization_url=STR)},
    ("post", "/api/v1/auth/oauth/{provider}/bind/start"): {"summary": "Get an authorization URL that binds the provider to the current account",
                                                           "response": S(authorization_url=STR)},
    ("get", "/api/v1/auth/oauth/{provider}/callback"): {
        "summary": "OAuth callback; on login sets session cookies",
        "response": S(account=REF("Account"), bound=BOOL, expires_at=DT)},

    # ---- chat -------------------------------------------------------
    ("get", "/api/v1/chat/channels"): {"response": data(ARR(REF("ChatChannel")))},
    ("post", "/api/v1/chat/lounge/messages"): {"request": S(body={"type": "string", "maxLength": 2000}),
                                               "response": data(REF("ChatMessage")), "status": "201"},
    ("post", "/api/v1/chat/channels/{channelKey}/messages"): {
        "request": S(body={"type": "string", "maxLength": 2000},
                     parent_id={**UUID, "description": "reply target (one level)"}),
        "response": data(REF("ChatMessage")), "status": "201"},
    ("patch", "/api/v1/chat/messages/{messageID}"): {"summary": "Edit your own website message",
                                                     "request": S(body={"type": "string", "maxLength": 2000}),
                                                     "response": data(REF("ChatMessage"))},
    ("delete", "/api/v1/chat/messages/{messageID}"): {"summary": "Withdraw your own website message", "status": "204"},
    ("post", "/api/v1/chat/messages/{messageID}/forward"): {"summary": "Forward a message into another channel",
                                                           "request": S(channel_key=STR),
                                                           "response": data(REF("ChatMessage")), "status": "201"},
    ("put", "/api/v1/chat/messages/{messageID}/reactions/{emoji}"): {"summary": "Add a reaction (URL-encode the emoji)", "status": "204"},
    ("delete", "/api/v1/chat/messages/{messageID}/reactions/{emoji}"): {"summary": "Remove a reaction", "status": "204"},
    ("get", "/api/v1/chat/channels/{channelKey}/pins"): {"response": data(ARR(REF("ChatMessage")))},
    ("post", "/api/v1/chat/messages/{messageID}/pin"): {"summary": "Pin a message (admin)", "status": "204"},
    ("delete", "/api/v1/chat/messages/{messageID}/pin"): {"summary": "Unpin a message (admin)", "status": "204"},
    ("post", "/api/v1/chat/webhooks/discord"): {"summary": "Discord relay webhook (HMAC signed)",
                                                "response": data(REF("ChatMessage"))},
    ("post", "/api/v1/chat/webhooks/telegram"): {"summary": "Telegram relay webhook (HMAC signed)",
                                                 "response": data(REF("ChatMessage"))},

    # ---- notifications -------------------------------------------
    ("get", "/api/v1/notifications/unread-count"): {"response": S(unread=INT)},
    ("post", "/api/v1/notifications/read-all"): {"response": S(marked_read=INT)},
    ("post", "/api/v1/notifications/{notificationID}/read"): {"status": "204"},

    # ---- profile -----------------------------------------------
    ("get", "/api/v1/profile"): {"response": data(REF("Profile"))},
    ("put", "/api/v1/profile"): {"summary": "Create or update your profile",
                                 "request": S(display_name={"type": "string", "maxLength": 80},
                                              bio={"type": "string", "maxLength": 500},
                                              links={**ARR(REF("ProfileLink")), "maxItems": 10}),
                                 "response": data(REF("Profile"))},
    ("post", "/api/v1/profile/avatar"): {"summary": "Upload an avatar (multipart `file`, PNG/JPEG <= 2 MB)",
                                         "requestContent": {"multipart/form-data": {"schema": S(file={"type": "string", "format": "binary"})}},
                                         "response": data(REF("Profile"))},
    ("delete", "/api/v1/profile/avatar"): {"summary": "Remove your avatar", "status": "204"},
    ("get", "/api/v1/profile/avatar"): {"summary": "302 redirect to a 5-minute presigned avatar URL", "status": "302"},
    ("get", "/api/v1/users/{username}"): {"response": data(REF("Profile"))},
    ("get", "/api/v1/users/{username}/avatar"): {"summary": "302 redirect to a presigned avatar URL", "status": "302"},

    # ---- applications --------------------------------------
    ("get", "/api/v1/applications"): {"response": data(ARR(REF("Application")))},
    ("post", "/api/v1/applications"): {"request": S(program_identifiers=ARR(STR)),
                                       "response": data(ARR(REF("Application"))), "status": "201"},
    ("put", "/api/v1/applications/{applicationID}/candidate-number"): {"request": S(candidate_number=STR), "status": "204"},
    ("put", "/api/v1/applications/{applicationID}/willingness"): {
        "request": S(value=INT, inquiry_id=UUID), "response": data(REF("WillingnessResponse"))},
    ("get", "/api/v1/applications/{applicationID}/result"): {"response": data(REF("ResultReport"))},
    ("get", "/api/v1/applications/{applicationID}/inquiries"): {"response": data(ARR(REF("ResultInquiry")))},
    ("post", "/api/v1/applications/service-tickets"): {"request": S(program_identifier=STR, reason=STR),
                                                      "response": data(REF("ServiceTicket")), "status": "201"},

    # ---- experiences / forum -----------------------------
    ("post", "/api/v1/experiences"): {"request": S(title=STR, body=STR, author_type=STR, admission_outcome=STR),
                                      "response": data(REF("Experience")), "status": "201"},
    ("get", "/api/v1/experiences/{experienceID}"): {"response": data(REF("Experience"))},
    ("post", "/api/v1/experiences/{experienceID}/revisions"): {"request": S(title=STR, body=STR, author_type=STR, admission_outcome=STR),
                                                              "response": data(REF("Experience")), "status": "201"},
    ("post", "/api/v1/experiences/{experienceID}/unpublish"): {"status": "204"},
    ("post", "/api/v1/experience-revisions/{revisionID}/submit"): {"response": data(REF("Experience"))},
    ("put", "/api/v1/experiences/{experienceID}/reactions/{emoji}"): {"summary": "Add a reaction to an experience", "status": "204"},
    ("delete", "/api/v1/experiences/{experienceID}/reactions/{emoji}"): {"summary": "Remove an experience reaction", "status": "204"},
    ("get", "/api/v1/forum/spaces"): {"response": data(ARR(REF("ForumSpace")))},
    ("post", "/api/v1/forum/spaces/{spaceID}/join"): {"status": "204"},
    ("post", "/api/v1/forum/spaces/{spaceID}/leave"): {"status": "204"},
    ("post", "/api/v1/forum/spaces/{spaceID}/threads"): {
        "request": S(title=STR, body=STR),
        "response": S(thread=REF("ForumThread"), post=REF("ForumPost")), "status": "201"},
    ("post", "/api/v1/forum/threads/{threadID}/posts"): {
        "request": S(body=STR, quoted_experience_id=UUID),
        "response": data(REF("ForumPost")), "status": "201"},
    ("put", "/api/v1/forum/posts/{postID}/reactions/{emoji}"): {"summary": "Add a reaction to a forum post", "status": "204"},
    ("delete", "/api/v1/forum/posts/{postID}/reactions/{emoji}"): {"summary": "Remove a forum-post reaction", "status": "204"},

    # ---- verification -------------------------------------
    ("get", "/api/v1/verification/requests"): {"response": data(ARR(REF("VerificationRequest")))},
    ("post", "/api/v1/verification/requests/school-email"): {
        "request": S(academic_year=INT, school_code=STR, program_code=STR, school_email={"type": "string", "format": "email"}),
        "response": S(request=REF("VerificationRequest"), code_expires_at=DT), "status": "201"},
    ("post", "/api/v1/verification/requests/{requestID}/verify-email"): {"request": S(code=STR),
                                                                        "response": data(REF("VerificationRequest"))},
    ("post", "/api/v1/verification/requests/document"): {"summary": "Create a document-upload verification request",
                                                        "request": S(academic_year=INT, school_code=STR, program_code=STR),
                                                        "response": data(REF("VerificationRequest")), "status": "201"},
    ("post", "/api/v1/verification/requests/{requestID}/documents"): {
        "summary": "Attach a document (multipart `file`)",
        "requestContent": {"multipart/form-data": {"schema": S(file={"type": "string", "format": "binary"})}},
        "response": data(REF("VerificationDocument")), "status": "201"},

    # ---- portfolio --------------------------------------
    ("get", "/api/v1/portfolio/projects"): {"response": data(ARR(REF("PortfolioProject")))},
    ("post", "/api/v1/portfolio/projects"): {"request": S(application_id=UUID, title=STR),
                                             "response": data(REF("PortfolioProject")), "status": "201"},
    ("get", "/api/v1/portfolio/projects/{projectID}/files"): {"response": data(ARR(REF("PortfolioFile")))},
    ("post", "/api/v1/portfolio/projects/{projectID}/files"): {
        "summary": "Upload a portfolio file (multipart `file`)",
        "requestContent": {"multipart/form-data": {"schema": S(file={"type": "string", "format": "binary"})}},
        "response": data(REF("PortfolioFile")), "status": "201"},
    ("get", "/api/v1/portfolio/files/{fileID}/download"): {"summary": "Short-lived signed URL", "response": PORTFOLIO_DL},
    ("get", "/api/v1/portfolio/files/{fileID}/events"): {"response": data(ARR(REF("PortfolioFileEvent")))},
    ("post", "/api/v1/portfolio/files/{fileID}/submit"): {"response": data(REF("PortfolioFile"))},
    ("post", "/api/v1/portfolio/files/{fileID}/unpublish"): {"response": data(REF("PortfolioFile"))},
    ("post", "/api/v1/portfolio/files/{fileID}/hide"): {"response": data(REF("PortfolioFile"))},

    # ---- support ---------------------------------------
    ("get", "/api/v1/support/tickets"): {"response": data(ARR(REF("SupportTicket")))},
    ("post", "/api/v1/support/tickets"): {"request": S(category=STR, subject=STR, body=STR),
                                          "response": data(REF("SupportTicketDetail")), "status": "201"},
    ("get", "/api/v1/support/tickets/{ticketID}"): {"response": data(REF("SupportTicketDetail"))},
    ("post", "/api/v1/support/tickets/{ticketID}/messages"): {"request": S(body=STR),
                                                             "response": data(REF("SupportMessage")), "status": "201"},
    ("post", "/api/v1/support/tickets/{ticketID}/close"): {"response": data(REF("SupportTicket"))},
    ("post", "/api/v1/support/tickets/{ticketID}/reopen"): {"response": data(REF("SupportTicket"))},
    ("get", "/api/v1/support/tickets/{ticketID}/attachments/{attachmentID}/download"): {"response": data(REF("SupportAttachment"))},
    ("post", "/api/v1/support/webhooks/discord"): {"summary": "Discord support relay (HMAC signed)", "response": S(ok=BOOL)},
    ("post", "/api/v1/support/webhooks/email"): {"summary": "Inbound support email (HMAC signed)", "response": S(ok=BOOL)},

    # ---- schools / admissions (public) --------------
    ("get", "/api/v1/schools"): {"response": data(ARR(REF("School")))},
    ("get", "/api/v1/admissions/schools"): {"response": data(ARR(S(school_code=STR, school_name=STR)))},
    ("get", "/api/v1/admissions/programs"): {"response": data(ARR(REF("AdmissionProgram")))},
    ("get", "/api/v1/admissions/programs/{identifier}"): {"response": data(REF("AdmissionProgram"))},
    ("get", "/api/v1/admissions/brochures/{academicYear}/{schoolCode}/download"): {"summary": "Signed URL for a published brochure PDF", "response": download_of(REF("BrochureDocument"))},

    # ---- meta / health ---------------------------------
    ("get", "/api/v1/meta"): {"response": S(api_version=STR, service=STR)},
    ("get", "/healthz"): {"response": S(status=STR)},
    ("get", "/readyz"): {"response": REF("Health")},
    ("get", "/api/v1/openapi.json"): {"summary": "This document"},

    # ---- admin: users / stats -------------------------
    ("get", "/api/v1/admin/stats"): {"summary": "Platform statistics snapshot", "response": REF("StatsSnapshot")},
    ("get", "/api/v1/admin/users/{accountID}"): {"response": data(REF("AdminUser"))},
    ("post", "/api/v1/admin/users/{accountID}/suspend"): {"request": S(reason={"type": "string", "minLength": 1, "maxLength": 500}),
                                                          "response": S(status=STR, sessions_revoked=INT)},
    ("post", "/api/v1/admin/users/{accountID}/reinstate"): {"request": _REASON, "response": S(status=STR)},
    ("post", "/api/v1/admin/users/{accountID}/force-logout"): {"request": _REASON, "response": S(sessions_revoked=INT)},

    # ---- admin: verification -------------------------
    ("get", "/api/v1/admin/verification/domains"): {"response": data(ARR(REF("VerificationDomain")))},
    ("post", "/api/v1/admin/verification/domains"): {"request": S(school_code=STR, domain=STR),
                                                    "response": data(REF("VerificationDomain")), "status": "201"},
    ("post", "/api/v1/admin/verification/domains/{domainID}/active"): {"request": S(is_active=BOOL),
                                                                      "response": data(REF("VerificationDomain"))},
    ("get", "/api/v1/admin/verification/requests/pending"): {"response": data(ARR(REF("VerificationRequest")))},
    ("get", "/api/v1/admin/verification/requests/{requestID}/documents"): {"response": data(ARR(REF("VerificationDocument")))},
    ("get", "/api/v1/admin/verification/requests/{requestID}/documents/{documentID}/download"): {"response": download_of(REF("VerificationDocument"))},
    ("post", "/api/v1/admin/verification/requests/{requestID}/review"): {
        "request": _APPROVE, "response": data(S(request=REF("VerificationRequest"), verification=REF("Verification")))},

    # ---- admin: support -----------------------------
    ("get", "/api/v1/admin/support/tickets"): {"response": data(ARR(REF("SupportTicket")))},
    ("get", "/api/v1/admin/support/tickets/{ticketID}"): {"response": data(REF("SupportTicketDetail"))},
    ("patch", "/api/v1/admin/support/tickets/{ticketID}"): {"request": S(status=STR, assigned_to=UUID),
                                                           "response": data(REF("SupportTicket"))},
    ("post", "/api/v1/admin/support/tickets/{ticketID}/messages"): {"request": S(body=STR),
                                                                   "response": data(REF("SupportMessage")), "status": "201"},
    ("post", "/api/v1/admin/support/tickets/{ticketID}/close"): {"response": data(REF("SupportTicket"))},
    ("get", "/api/v1/admin/support/tickets/{ticketID}/attachments/{attachmentID}/download"): {"response": data(REF("SupportAttachment"))},

    # ---- admin: portfolio ---------------------------
    ("get", "/api/v1/admin/portfolio/files"): {"response": data(ARR(REF("PortfolioFile")))},
    ("get", "/api/v1/admin/portfolio/files/{fileID}/events"): {"response": data(ARR(REF("PortfolioFileEvent")))},
    ("post", "/api/v1/admin/portfolio/files/{fileID}/review"): {"request": _APPROVE, "response": data(REF("PortfolioFile"))},

    # ---- admin: schools -----------------------------
    ("get", "/api/v1/admin/schools"): {"response": data(ARR(REF("School")))},
    ("get", "/api/v1/admin/schools/{schoolCode}/history"): {"response": data(ARR(REF("SchoolAuditEvent")))},
    ("post", "/api/v1/admin/schools/sync"): {"summary": "Batch upsert the school master",
                                             "request": S(reason=STR, items=ARR(S(school_code=STR, school_name=STR, institution_type=STR, is_active=BOOL))),
                                             "response": data(ARR(REF("School")))},
    ("put", "/api/v1/admin/schools/{schoolCode}"): {"request": S(school_name=STR, institution_type=STR, is_active=BOOL),
                                                   "response": data(REF("School"))},

    # ---- admin: admissions programs / brochures -----
    ("get", "/api/v1/admin/admissions/programs"): {"response": data(ARR(REF("AdmissionProgram")))},
    ("get", "/api/v1/admin/admissions/programs/{identifier}"): {"response": data(REF("AdmissionProgram"))},
    ("get", "/api/v1/admin/admissions/programs/{identifier}/history"): {"response": data(ARR(REF("ProgramAuditEvent")))},
    ("post", "/api/v1/admin/admissions/programs/sync"): {"summary": "Batch upsert programs (re-enter pending)",
                                                        "request": S(reason=STR, items=ARR(REF("AdmissionProgram"))),
                                                        "response": data(ARR(REF("AdmissionProgram")))},
    ("put", "/api/v1/admin/admissions/programs/{identifier}"): {"request": S(reason=STR, item=REF("AdmissionProgram")),
                                                              "response": data(REF("AdmissionProgram"))},
    ("post", "/api/v1/admin/admissions/programs/{identifier}/review"): {"request": _APPROVE,
                                                                       "response": data(REF("AdmissionProgram"))},
    ("get", "/api/v1/admin/admissions/brochures"): {"response": data(ARR(REF("BrochureDocument")))},
    ("post", "/api/v1/admin/admissions/brochures"): {"summary": "Upload an official brochure PDF (multipart)",
                                                    "requestContent": {"multipart/form-data": {"schema": S(
                                                        file={"type": "string", "format": "binary"},
                                                        academic_year=STR, school_code=STR, source_url=STR)}},
                                                    "response": data(REF("BrochureDocument")), "status": "201"},
    ("get", "/api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/download"): {"response": download_of(REF("BrochureDocument"))},
    ("get", "/api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/events"): {"response": data(ARR(S(action=STR, reason=STR, created_at=DT)))},
    ("post", "/api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/review"): {"request": _APPROVE,
                                                                                      "response": data(REF("BrochureDocument"))},
    ("post", "/api/v1/admin/admissions/brochures/{academicYear}/{schoolCode}/visibility"): {"request": S(visible=BOOL),
                                                                                          "response": data(REF("BrochureDocument"))},

    # ---- admin: admission sources -------------------
    ("get", "/api/v1/admin/admission-sources"): {"response": data(ARR(REF("AdmissionSource")))},
    ("post", "/api/v1/admin/admission-sources"): {"request": REF("AdmissionSource"),
                                                 "response": data(REF("AdmissionSource")), "status": "201"},
    ("patch", "/api/v1/admin/admission-sources/{sourceID}"): {"request": REF("AdmissionSource"),
                                                            "response": data(REF("AdmissionSource"))},

    # ---- admin: ingestion --------------------------
    ("get", "/api/v1/admin/ingestion/brochure-runs"): {"response": data(ARR(REF("IngestionRun")))},
    ("get", "/api/v1/admin/ingestion/brochure-runs/{runID}"): {"response": data(REF("IngestionRun"))},
    ("post", "/api/v1/admin/ingestion/brochure-runs/{runID}/review"): {"request": _APPROVE, "response": data(REF("IngestionRun"))},
    ("get", "/api/v1/admin/ingestion/jobs/{jobID}"): {"response": data(REF("IngestionJobStatus"))},
    ("post", "/api/v1/admin/ingestion/jobs/{jobID}/retry"): {"response": data(REF("IngestionJobStatus"))},
    ("post", "/api/v1/admin/ingestion/brochure-candidates/{candidateID}/review"): {"request": _APPROVE,
                                                                                 "response": data(REF("IngestionCandidate"))},

    # ---- admin: results ---------------------------
    ("post", "/api/v1/admin/results/import"): {"summary": "Import an official-result batch",
                                              "request": S(academic_year=INT, school_code=STR, source_url=STR,
                                                           source_sha256=STR, rows=ARR({"type": "object"})),
                                              "response": S(batch_id=UUID), "status": "201"},
    ("get", "/api/v1/admin/results/batches"): {"response": data(ARR(REF("AdminResultBatch")))},
    ("get", "/api/v1/admin/results/batches/{batchID}"): {"response": data(REF("AdminResultBatch"))},
    ("post", "/api/v1/admin/results/{batchID}/publish"): {"response": data(REF("AdminResultBatch"))},
    ("post", "/api/v1/admin/results/{batchID}/inquiries/acceptance-deadline"): {"request": S(round=STR, response_deadline=DT),
                                                                              "response": data(REF("AdminResultBatch"))},
    ("post", "/api/v1/admin/results/{resultID}/correct"): {"request": S(result_status=STR, official_rank=INT, quota=INT,
                                                                        masked_name=STR, reason=STR),
                                                          "response": S(corrected=BOOL)},

    # ---- admin: misc ------------------------------
    ("post", "/api/v1/admin/experience-revisions/{revisionID}/review"): {"request": _APPROVE, "response": data(REF("Experience"))},
    ("get", "/api/v1/admin/applications/service-tickets"): {"response": data(ARR(REF("ServiceTicket")))},
    ("post", "/api/v1/admin/applications/service-tickets/{ticketID}/review"): {"request": _APPROVE,
                                                                             "response": data(REF("ServiceTicket"))},
    ("post", "/api/v1/admin/search/reindex"): {"summary": "Rebuild the Meilisearch indexes", "response": S(reindexed=BOOL)},
    ("post", "/api/v1/admin/extraction/candidate-lists"): {"summary": "Submit a candidate-list extraction result (agent)",
                                                          "response": S(accepted=BOOL)},
    ("get", "/api/v1/admin/telegram-cross-check/status"): {"response": data(REF("CrossCheckAdminStatus"))},
    ("post", "/api/v1/admin/telegram-cross-check/participants/sync"): {
        "request": S(reason=STR, participants=ARR({"type": "object"})),
        "response": data(ARR({"type": "object"}))},

    # ---- search ----------------------------------
    ("get", "/api/v1/search"): {"summary": "Meilisearch multi-search grouped by index",
                                "response": data({"type": "object", "additionalProperties": ARR({"type": "object"})})},

    # ---- brochure discovery (admin console + agent share the shapes) --
    ("get", "/api/v1/admin/admissions/brochure-discovery/cycles"): {"response": data(ARR(REF("DiscoveryCycle")))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles"): {"request": S(academic_year=INT),
                                                                    "response": data(REF("DiscoveryCycle")), "status": "201"},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/start"): {"response": data(REF("DiscoveryCycle"))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/close"): {"response": data(REF("DiscoveryCycle"))},
    ("get", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks"): {"response": data(ARR(REF("DiscoveryTask")))},
    ("get", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/events"): {"response": data(ARR(REF("DiscoveryEvent")))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/claim"): {"summary": "Claim the next discovery task (agent)",
                                                                   "response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/candidate"): {
        "request": S(detected_academic_year=INT, source_url=STR, document_url=STR, sha256=STR, confidence=NUM,
                     evidence={"type": "object"}), "response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/failure"): {
        "request": S(code=STR, message=STR), "response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/no-brochure"): {"response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/retry"): {"response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/manual-complete"): {
        "request": S(source_url=STR, document_url=STR), "response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/review"): {
        "request": _APPROVE, "response": data(REF("DiscoveryTask"))},

    ("post", "/api/v1/internal/admissions/brochure-discovery/claim"): {"summary": "Claim the next discovery task (agent)",
                                                                      "response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/candidate"): {
        "request": S(detected_academic_year=INT, source_url=STR, document_url=STR, sha256=STR, confidence=NUM,
                     evidence={"type": "object"}), "response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/failure"): {
        "request": S(code=STR, message=STR), "response": data(REF("DiscoveryTask"))},
    ("post", "/api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/no-match"): {"response": data(REF("DiscoveryTask"))},

    # ---- internal: extraction (Python worker + agents) --------------
    ("post", "/api/v1/internal/extraction/jobs/claim"): {"summary": "Claim the next extraction job",
                                                        "response": data(REF("ExtractionClaim"))},
    ("get", "/api/v1/internal/extraction/jobs/{jobID}"): {"response": data(REF("ExtractionClaim"))},
    ("post", "/api/v1/internal/extraction/jobs/{jobID}/result"): {"summary": "Submit an extraction result",
                                                                "response": data(S(job_id=UUID, status=STR))},
    ("post", "/api/v1/internal/extraction/jobs/{jobID}/failure"): {"request": S(code=STR, message=STR, retryable=BOOL),
                                                                 "response": data(S(job_id=UUID, status=STR))},
    ("post", "/api/v1/internal/extraction/brochures"): {"summary": "Submit a brochure-extraction result (agent)",
                                                       "response": data({"type": "object"})},
    ("post", "/api/v1/internal/extraction/candidate-lists"): {"summary": "Submit a candidate-list extraction result (agent)",
                                                            "response": data({"type": "object"})},

    # ---- internal: telegram cross-check ---------------------------
    ("post", "/api/v1/internal/telegram-cross-check/bind"): {"request": S(telegram_user_id={"type": "integer", "format": "int64"},
                                                                        private_chat_id={"type": "integer", "format": "int64"}),
                                                           "response": data(REF("CrossCheckDashboard"))},
    ("post", "/api/v1/internal/telegram-cross-check/disable"): {"status": "204"},
    ("post", "/api/v1/internal/telegram-cross-check/respond"): {"request": S(inquiry_id=UUID, value=INT),
                                                             "response": data({"type": "object"})},
    ("get", "/api/v1/internal/telegram-cross-check/users/{telegramUserID}/dashboard"): {"response": data(REF("CrossCheckDashboard"))},
    ("get", "/api/v1/internal/telegram-cross-check/users/{telegramUserID}/history"): {"response": data(ARR({"type": "object"}))},
    ("post", "/api/v1/internal/telegram-cross-check/outbox/claim"): {"response": data(ARR({"type": "object"}))},
    ("post", "/api/v1/internal/telegram-cross-check/outbox/{deliveryID}/sent"): {"status": "204"},
    ("post", "/api/v1/internal/telegram-cross-check/outbox/{deliveryID}/failed"): {"request": S(message=STR), "status": "204"},
}


# --------------------------------------------------------------------------- #
#  Route -> operation
# --------------------------------------------------------------------------- #

_PUBLIC_AUTH = {
    "/api/v1/auth/register", "/api/v1/auth/login",
    "/api/v1/auth/password-reset/request", "/api/v1/auth/password-reset/confirm",
    "/api/v1/auth/email-verification/confirm",
    "/api/v1/auth/oauth/{provider}/start", "/api/v1/auth/oauth/{provider}/callback",
}


def tag_for(path: str) -> str:
    parts = [p for p in path.split("/") if p and not p.startswith("{")]
    if parts[:2] == ["api", "v1"]:
        if len(parts) >= 3 and parts[2] == "admin":
            return "admin-" + (parts[3] if len(parts) > 3 else "misc")
        if len(parts) >= 3 and parts[2] == "internal":
            return "internal-" + (parts[3] if len(parts) > 3 else "misc")
        return parts[2] if len(parts) > 2 else "meta"
    return "meta"


def security_for(method: str, path: str) -> list[dict]:
    mutating = method in ("post", "put", "patch", "delete")
    if "/api/v1/internal/" in path:
        return [{"serviceToken": []}]
    if path in ("/healthz", "/readyz", "/metrics") or path.endswith("/openapi.json") or path == "/api/v1/meta":
        return []
    if "/webhooks/" in path:
        return [{"webhookSignature": []}]
    if path in _PUBLIC_AUTH:
        return []
    if "/api/v1/admin/" in path:
        cookie = {"sessionCookie": [], "adminMFA": []}
        token = {"bearer": [], "adminMFA": []}
        if mutating:
            cookie["csrf"] = []
        return [cookie, token]
    cookie = {"sessionCookie": []}
    if mutating:
        cookie["csrf"] = []
    return [cookie, {"bearer": []}]


def summarize(method: str, path: str) -> str:
    tail = [p for p in path.split("/") if p]
    verb = {"get": "Get", "post": "Create", "put": "Replace", "patch": "Update", "delete": "Delete"}[method]
    noun = " ".join(s.strip("{}").replace("-", " ") for s in tail[2:] or tail)
    return f"{verb} {noun}".strip()


def json_body(schema: dict) -> dict:
    return {"content": {"application/json": {"schema": schema}}}


def build_op(method: str, path: str) -> dict:
    ov = ROUTES.get((method, path), {})
    keyset = (method, path) in KEYSET

    op: dict = {
        "operationId": f"{method}_" + re.sub(r"[^a-zA-Z0-9]+", "_", path).strip("_"),
        "summary": ov.get("summary") or summarize(method, path),
        "tags": [tag_for(path)],
    }

    params: list[dict] = [
        {"name": n, "in": "path", "required": True, "schema": {"type": "string"}}
        for n in re.findall(r"\{([^}]+)\}", path)
    ]
    if keyset:
        params += [_LIMIT, _CURSOR]
    params += QUERY.get((method, path), [])
    if params:
        op["parameters"] = params

    sec = security_for(method, path)
    if sec:
        op["security"] = sec

    if "requestContent" in ov:
        op["requestBody"] = {"required": True, "content": ov["requestContent"]}
    elif method in ("post", "put", "patch"):
        op["requestBody"] = json_body(ov.get("request", {"type": "object"}))

    status = ov.get("status", "200")
    responses: dict = {}
    if status == "204":
        responses["204"] = {"description": "No Content"}
    elif status == "302":
        responses["302"] = {"description": "Redirect to a presigned URL (Location header)"}
    elif method == "get" and path == "/api/v1/events":
        responses["200"] = {"description": "SSE stream",
                            "content": {"text/event-stream": {"schema": {"type": "string"}}}}
    else:
        if keyset:
            item = KEYSET[(method, path)]
            body = page(REF(item) if item else {"type": "object"})
        else:
            body = ov.get("response", {"type": "object"})
        responses[status] = {"description": "OK", **json_body(body)}
    responses["4XX"] = {"description": "Client error", **json_body(REF("Error"))}
    op["responses"] = responses
    return op


def main() -> int:
    routes: set[tuple[str, str]] = set()
    for go in (ROOT / "internal").rglob("*.go"):
        if go.name.endswith("_test.go"):
            continue
        for line in go.read_text(encoding="utf-8").splitlines():
            code = line.split("//", 1)[0]  # ignore commented-out / illustrative routes
            for m in ROUTE_RE.finditer(code):
                routes.add((m.group(1).lower(), m.group(2)))

    paths: dict[str, dict] = {}
    for method, path in sorted(routes, key=lambda r: (r[1], r[0])):
        paths.setdefault(path, {})[method] = build_op(method, path)

    typed = sum(
        1
        for node in paths.values()
        for op in node.values()
        for code, r in op["responses"].items()
        if code != "4XX" and (
            code in ("204", "302")
            or "$ref" in json.dumps(r)
            or r.get("content", {}).get("application/json", {}).get("schema", {}) != {"type": "object"}
        )
    )

    spec = {
        "openapi": "3.1.0",
        "info": {
            "title": "STA Platform API",
            "version": "v1",
            "description": (
                "Generated from the route literals in internal/**/*.go. Method, path, tags "
                "and the method-aware security block are machine-accurate; request/response "
                "schemas are hand-derived from the handler shapes and kept in this generator. "
                "Regenerate with `python3 docs/openapi/generate.py`."
            ),
        },
        "servers": [{"url": "http://localhost:8080", "description": "local"}],
        "components": {
            "securitySchemes": {
                "sessionCookie": {"type": "apiKey", "in": "cookie", "name": "sta_session"},
                "csrf": {"type": "apiKey", "in": "header", "name": "X-CSRF-Token"},
                "bearer": {"type": "http", "scheme": "bearer"},
                "adminMFA": {"type": "apiKey", "in": "header", "name": "X-MFA-Code"},
                "serviceToken": {"type": "http", "scheme": "bearer", "description": "extraction / agent service token"},
                "webhookSignature": {"type": "apiKey", "in": "header", "name": "X-STA-Signature",
                                     "description": "HMAC-SHA256 of the raw body"},
            },
            "schemas": SCHEMAS,
        },
        "paths": paths,
    }

    payload = json.dumps(spec, indent=2, ensure_ascii=False) + "\n"
    for rel in ("docs/openapi.json", "internal/httpapi/openapi.json"):
        (ROOT / rel).write_text(payload, encoding="utf-8")
    n_ops = sum(len(v) for v in paths.values())
    print(f"wrote docs/openapi.json + internal/httpapi/openapi.json  "
          f"({len(paths)} paths, {n_ops} operations, {typed} with concrete responses)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
