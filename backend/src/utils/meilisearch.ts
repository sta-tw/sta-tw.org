export interface MeiliConfig {
  url: string
  masterKey: string
}

async function meiliRequest(
  config: MeiliConfig,
  method: string,
  path: string,
  body?: unknown,
): Promise<unknown> {
  const res = await fetch(`${config.url}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${config.masterKey}`,
      'Content-Type': 'application/json',
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error(`Meilisearch error: ${res.status}`)
  return res.json()
}

export async function searchMulti(
  config: MeiliConfig,
  query: string,
): Promise<{ messages: unknown[]; channels: unknown[]; users: unknown[] }> {
  try {
    const result = await meiliRequest(config, 'POST', '/multi-search', {
      queries: [
        { indexUid: 'messages', q: query, limit: 20 },
        { indexUid: 'channels', q: query, limit: 10 },
        { indexUid: 'users', q: query, limit: 10 },
      ],
    }) as { results: Array<{ hits: unknown[] }> }
    const [msgs, chs, usrs] = result.results
    return { messages: msgs?.hits ?? [], channels: chs?.hits ?? [], users: usrs?.hits ?? [] }
  } catch {
    return { messages: [], channels: [], users: [] }
  }
}

export async function indexMessage(config: MeiliConfig, doc: Record<string, unknown>): Promise<void> {
  try {
    await meiliRequest(config, 'POST', '/indexes/messages/documents', [doc])
  } catch { /* best-effort */ }
}

export async function deleteMessageFromIndex(config: MeiliConfig, id: string): Promise<void> {
  try {
    await meiliRequest(config, 'DELETE', `/indexes/messages/documents/${id}`)
  } catch { /* best-effort */ }
}
