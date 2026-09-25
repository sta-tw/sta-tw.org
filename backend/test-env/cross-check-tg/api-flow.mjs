#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

const apiBase = requiredEnv("STA_PUBLIC_BASE_URL").replace(/\/$/, "");
const serviceToken = requiredEnv("STA_TELEGRAM_CROSS_CHECK_TOKEN");
const adminUsername = requiredEnv("CROSS_CHECK_TEST_ADMIN_USERNAME");
const adminEmail = requiredEnv("CROSS_CHECK_TEST_ADMIN_EMAIL");
const adminPassword = requiredEnv("CROSS_CHECK_TEST_ADMIN_PASSWORD");
const command = process.argv[2] ?? "help";

if (command === "register-admin") {
  await registerAdmin();
} else if (command === "flow" || command === "verify") {
  const roster = await loadRoster(process.argv[3]);
  const session = await loginAdmin();
  if (command === "flow") {
    await ensurePrograms(session, roster.programs);
    await syncParticipants(session, roster.participants);
    await importAndPublish(session, roster);
  }
  await verifySourceState(session, roster);
} else if (command === "status") {
  const session = await loginAdmin();
  const response = await apiRequest("/api/v1/admin/telegram-cross-check/status", { session });
  process.stdout.write(`${JSON.stringify(response.payload.data, null, 2)}\n`);
} else if (command === "simulate-starts") {
  const roster = await loadRoster(process.argv[3]);
  await simulateStarts(roster);
} else if (command === "simulate-response") {
  const roster = await loadRoster(process.argv[3]);
  await simulateResponse(roster);
} else if (command === "probe-outbox") {
  await probeOutbox();
} else {
  process.stdout.write("用法：node api-flow.mjs <register-admin|flow|verify|status|simulate-starts|simulate-response|probe-outbox> [roster.json]\n");
  process.exitCode = command === "help" ? 0 : 2;
}

async function registerAdmin() {
  const response = await apiRequest("/api/v1/auth/register", {
    method: "POST",
    body: { username: adminUsername, email: adminEmail, password: adminPassword },
    expected: [201, 409]
  });
  if (response.status === 201) {
    process.stdout.write(`已透過來源 API 建立隔離管理員帳號 ${adminUsername}。\n`);
  } else {
    process.stdout.write(`隔離帳號 ${adminUsername} 已存在；未建立替代帳號。\n`);
  }
}

async function loginAdmin() {
  const response = await apiRequest("/api/v1/auth/login", {
    method: "POST",
    body: { username: adminUsername, password: adminPassword },
    expected: [200]
  });
  const rawCookies = typeof response.headers.getSetCookie === "function"
    ? response.headers.getSetCookie().join(",")
    : (response.headers.get("set-cookie") ?? "");
  const sessionToken = extractCookie(rawCookies, "sta_session");
  const csrfToken = extractCookie(rawCookies, "sta_csrf");
  if (!sessionToken || !csrfToken) {
    throw new Error("登入成功但來源 API 未回傳完整 session／CSRF cookie");
  }
  return {
    cookie: `sta_session=${sessionToken}; sta_csrf=${csrfToken}`,
    csrf: csrfToken
  };
}

async function ensurePrograms(session, programs) {
  assertArray(programs, "programs");
  for (const program of programs) {
    const identifier = programIdentifier(program);
    let current = await apiRequest(`/api/v1/admin/admissions/programs/${identifier}`, {
      session,
      expected: [200, 404]
    });
    if (current.status === 404) {
      const item = {
        academic_year: program.academic_year,
        school_code: program.school_code,
        program_code: program.program_code,
        admission_program_name: program.admission_program_name,
        admission_quota: program.admission_quota,
        exam_items: program.exam_items ?? [{
          name: "資料審查",
          sort_order: 1,
          weight_percent: 100,
          description: "來源 API 隔離驗收",
          source_page: String(program.source_page ?? 1)
        }],
        source_page: program.source_page ?? 1,
        notes: "來源 API 隔離驗收資料"
      };
      const synced = await apiRequest("/api/v1/admin/admissions/programs/sync", {
        method: "POST",
        session,
        body: { reason: "交叉查榜 Telegram API 隔離驗收", items: [item] },
        expected: [200]
      });
      current = { status: 200, payload: { data: synced.payload.data[0] } };
    }
    const reviewStatus = current.payload.data.review_status;
    if (reviewStatus === "pending") {
      current = await apiRequest(`/api/v1/admin/admissions/programs/${identifier}/review`, {
        method: "POST",
        session,
        body: { approved: true, reason: "交叉查榜 Telegram API 隔離驗收" },
        expected: [200]
      });
    }
    if (current.payload.data.review_status !== "published") {
      throw new Error(`校系 ${identifier} 不是 published，實際狀態為 ${current.payload.data.review_status}`);
    }
    process.stdout.write(`來源 API 校系已上架：${identifier}\n`);
  }
}

async function syncParticipants(session, participants) {
  assertArray(participants, "participants");
  const response = await apiRequest("/api/v1/admin/telegram-cross-check/participants/sync", {
    method: "POST",
    session,
    body: {
      reason: "交叉查榜 Telegram API 隔離驗收名單",
      participants: participants.map((participant) => ({
        telegram_user_id: participant.telegram_user_id,
        assignments: participant.assignments.map((assignment) => ({
          program_identifier: assignment.program_identifier,
          candidate_number: assignment.candidate_number
        }))
      }))
    },
    expected: [200]
  });
  if (response.payload.meta.participant_count !== participants.length) {
    throw new Error("來源 API 回傳的參與者數量與 roster 不一致");
  }
  process.stdout.write(`來源 API 已同步 ${participants.length} 位 Telegram 測試參與者。\n`);
}

async function importAndPublish(session, roster) {
  const groups = resultGroups(roster);
  for (const group of groups.values()) {
    const source = roster.result_sources.find((candidate) =>
      candidate.academic_year === group.academicYear && candidate.school_code === group.schoolCode
    );
    if (!source) {
      throw new Error(`缺少 ${group.academicYear}-${group.schoolCode} 的 result_sources 設定`);
    }
    const listed = await apiRequest(`/api/v1/admin/results/batches?academic_year=${group.academicYear}&school_code=${group.schoolCode}&limit=100`, { session });
    let batch = listed.payload.data.find((candidate) => candidate.source_sha256 === source.source_sha256);
    if (!batch) {
      const imported = await apiRequest("/api/v1/admin/results/import", {
        method: "POST",
        session,
        body: {
          academic_year: group.academicYear,
          school_code: group.schoolCode,
          source_url: source.source_url,
          source_sha256: source.source_sha256,
          rows: group.rows
        },
        expected: [201]
      });
      batch = { id: imported.payload.batch_id, status: "pending_review" };
      process.stdout.write(`來源 API 已匯入榜單批次：${batch.id}\n`);
    }
    if (batch.status === "pending_review") {
      await apiRequest(`/api/v1/admin/results/${batch.id}/publish`, {
        method: "POST",
        session,
        body: {},
        expected: [204]
      });
      process.stdout.write(`來源 API 已發布榜單批次：${batch.id}\n`);
    } else if (batch.status !== "published") {
      throw new Error(`榜單批次 ${batch.id} 無法發布，狀態為 ${batch.status}`);
    }
  }
}

async function verifySourceState(session, roster) {
  const groups = resultGroups(roster);
  let expectedInquiries = 0;
  for (const group of groups.values()) {
    expectedInquiries += group.rows.filter((row) => row.result_status === "admitted" || row.result_status === "waitlisted").length;
    const source = roster.result_sources.find((candidate) =>
      candidate.academic_year === group.academicYear && candidate.school_code === group.schoolCode
    );
    const listed = await apiRequest(`/api/v1/admin/results/batches?academic_year=${group.academicYear}&school_code=${group.schoolCode}&limit=100`, { session });
    const batch = listed.payload.data.find((candidate) => candidate.source_sha256 === source.source_sha256);
    if (!batch || batch.status !== "published") {
      throw new Error(`${group.academicYear}-${group.schoolCode} 沒有符合 roster checksum 的 published 榜單`);
    }
    const detail = await apiRequest(`/api/v1/admin/results/batches/${batch.id}`, { session });
    if (detail.payload.data.batch.matched_count !== group.rows.length) {
      throw new Error(`榜單 ${batch.id} 實際匹配 ${detail.payload.data.batch.matched_count} 筆，預期 ${group.rows.length} 筆`);
    }
  }
  const status = await apiRequest("/api/v1/admin/telegram-cross-check/status", { session });
  const data = status.payload.data;
  const outboxCount = Object.values(data.outbox_by_status).reduce((sum, value) => sum + value, 0);
  if (data.participant_count !== roster.participants.length) {
    throw new Error(`Telegram 參與者實際 ${data.participant_count} 位，預期 ${roster.participants.length} 位`);
  }
  if (outboxCount !== expectedInquiries) {
    throw new Error(`Telegram outbox 實際 ${outboxCount} 筆，預期 ${expectedInquiries} 筆`);
  }
  process.stdout.write(`API 驗收通過：${data.participant_count} 位參與者、${outboxCount} 筆真實詢問 outbox。\n`);
  process.stdout.write(`outbox 狀態：${JSON.stringify(data.outbox_by_status)}\n`);
}

async function simulateStarts(roster) {
  for (const participant of roster.participants) {
    const id = participant.telegram_user_id;
    await internalRequest("/api/v1/internal/telegram-cross-check/bind", {
      method: "POST",
      body: { telegram_user_id: id, private_chat_id: id },
      expected: [204]
    });
    const dashboard = await internalRequest(`/api/v1/internal/telegram-cross-check/users/${id}/dashboard`);
    if (dashboard.payload.data.applications.length !== participant.assignments.length) {
      throw new Error(`Telegram ID ${id} 的 dashboard 數量與 roster 不一致`);
    }
    process.stdout.write(`已模擬 Telegram ID ${id} 執行 /start；outbox 現在可由真實 Bot 領取。\n`);
  }
}

async function simulateResponse(roster) {
  const telegramUserID = roster.participants[0].telegram_user_id;
  const dashboard = await internalRequest(`/api/v1/internal/telegram-cross-check/users/${telegramUserID}/dashboard`);
  const target = dashboard.payload.data.applications.find((application) => application.pending_inquiry);
  if (!target) {
    throw new Error(`Telegram ID ${telegramUserID} 沒有可供 API response 驗收的 pending inquiry`);
  }
  const callbackID = `api-harness-${target.pending_inquiry.id}`;
  const body = {
    telegram_user_id: telegramUserID,
    inquiry_id: target.pending_inquiry.id,
    choice: "considering",
    callback_id: callbackID
  };
  const first = await internalRequest("/api/v1/internal/telegram-cross-check/respond", {
    method: "POST",
    body
  });
  const duplicate = await internalRequest("/api/v1/internal/telegram-cross-check/respond", {
    method: "POST",
    body
  });
  if (first.payload.data.choice_label !== "還在考慮" || duplicate.payload.data.choice_label !== "還在考慮") {
    throw new Error("來源 API 沒有把意見代碼轉成預期文字標籤");
  }
  const history = await internalRequest(`/api/v1/internal/telegram-cross-check/users/${telegramUserID}/history?limit=20`);
  const matchingEvents = history.payload.data.filter((event) => event.application_id === target.application_id);
  if (matchingEvents.length !== 1 || matchingEvents[0].choice_label !== "還在考慮") {
    throw new Error(`相同 callback 應只有一筆歷程，實際為 ${matchingEvents.length} 筆`);
  }
  process.stdout.write(`來源 API 回覆與冪等驗收通過：Telegram ID ${telegramUserID}，歷程維持 1 筆。\n`);
}

async function probeOutbox() {
  const claimed = await internalRequest("/api/v1/internal/telegram-cross-check/outbox/claim", {
    method: "POST",
    body: { limit: 25 }
  });
  if (!Array.isArray(claimed.payload.data) || claimed.payload.data.length === 0) {
    throw new Error("沒有可領取的 pending Telegram outbox；請先執行 simulate-starts")
  }
  for (const delivery of claimed.payload.data) {
    await internalRequest(`/api/v1/internal/telegram-cross-check/outbox/${delivery.id}/failed`, {
      method: "POST",
      body: {
        error: "API harness probe; no Telegram API call was made",
        retryable: true
      },
      expected: [204]
    });
  }
  process.stdout.write(`來源 API outbox claim／failed 驗收通過：${claimed.payload.data.length} 筆；沒有冒充 Telegram 已送達。\n`);
}

function resultGroups(roster) {
  const groups = new Map();
  for (const participant of roster.participants) {
    for (const assignment of participant.assignments) {
      const match = /^(\d{3})-(\d{3})-(\d{3})$/.exec(assignment.program_identifier);
      if (!match) {
        throw new Error(`無效的 program_identifier：${assignment.program_identifier}`);
      }
      const academicYear = Number(match[1]);
      const schoolCode = match[2];
      const programCode = match[3];
      const key = `${academicYear}-${schoolCode}`;
      if (!groups.has(key)) {
        groups.set(key, { academicYear, schoolCode, rows: [] });
      }
      groups.get(key).rows.push({
        academic_year: academicYear,
        school_code: schoolCode,
        program_code: programCode,
        candidate_number: assignment.candidate_number,
        masked_name: assignment.masked_name ?? "-",
        result_status: assignment.result_status,
        official_rank: assignment.official_rank ?? null,
        quota: assignment.quota ?? null,
        source_page: assignment.source_page ?? 1
      });
    }
  }
  return groups;
}

async function loadRoster(rawPath) {
  if (!rawPath) {
    throw new Error("請提供 roster JSON 路徑");
  }
  const path = resolve(rawPath);
  const roster = JSON.parse(await readFile(path, "utf8"));
  assertArray(roster.programs, "programs");
  assertArray(roster.result_sources, "result_sources");
  assertArray(roster.participants, "participants");
  return roster;
}

function programIdentifier(program) {
  const year = String(program.academic_year).padStart(3, "0");
  return `${year}-${program.school_code}-${program.program_code}`;
}

async function internalRequest(path, options = {}) {
  return apiRequest(path, {
    ...options,
    headers: { ...(options.headers ?? {}), Authorization: `Bearer ${serviceToken}` }
  });
}

async function apiRequest(path, options = {}) {
  const method = options.method ?? "GET";
  const headers = { Accept: "application/json", ...(options.headers ?? {}) };
  if (options.session) {
    headers.Cookie = options.session.cookie;
    if (method !== "GET" && method !== "HEAD") {
      headers["X-CSRF-Token"] = options.session.csrf;
    }
  }
  let body;
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(options.body);
  }
  const response = await fetch(apiBase + path, { method, headers, body, redirect: "manual" });
  const text = await response.text();
  let payload = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      throw new Error(`${method} ${path} 回傳非 JSON 內容（HTTP ${response.status}）`);
    }
  }
  const expected = options.expected ?? [200];
  if (!expected.includes(response.status)) {
    const code = payload?.error?.code ?? "unknown_error";
    const message = payload?.error?.message ?? "沒有錯誤說明";
    throw new Error(`${method} ${path} 失敗（HTTP ${response.status}, ${code}）：${message}`);
  }
  return { status: response.status, payload, headers: response.headers };
}

function extractCookie(raw, name) {
  const match = new RegExp(`(?:^|[,\\s])${name}=([^;,\\s]+)`).exec(raw);
  return match?.[1] ?? "";
}

function assertArray(value, label) {
  if (!Array.isArray(value) || value.length === 0) {
    throw new Error(`${label} 必須是非空陣列`);
  }
}

function requiredEnv(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`缺少環境變數 ${name}`);
  }
  return value;
}
