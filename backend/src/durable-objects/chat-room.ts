export class ChatRoom {
  private state: DurableObjectState
  private sessions = new Map<WebSocket, Set<string>>()

  constructor(state: DurableObjectState) {
    this.state = state
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url)

    // Internal broadcast from worker
    if (url.pathname === '/broadcast' && request.method === 'POST') {
      const { channelId, event } = await request.json() as { channelId: string; event: unknown }
      this.broadcastToChannel(channelId, event)
      return new Response('ok')
    }

    if (request.headers.get('Upgrade') !== 'websocket') {
      return new Response('Expected WebSocket', { status: 426 })
    }

    const pair = new WebSocketPair()
    const [client, server] = Object.values(pair) as [WebSocket, WebSocket]
    server.accept()
    this.sessions.set(server, new Set())

    server.addEventListener('message', (event) => {
      try {
        const data = JSON.parse(event.data as string)
        const channelIds = this.sessions.get(server)
        if (!channelIds) return

        if (data.type === 'subscribe' && data.channelId) {
          channelIds.add(data.channelId)
        } else if (data.type === 'unsubscribe' && data.channelId) {
          channelIds.delete(data.channelId)
        } else if (data.type === 'typing' && data.channelId) {
          this.broadcastToChannel(
            data.channelId,
            { type: 'typing', channelId: data.channelId, userId: data.userId },
            server,
          )
        }
      } catch { /* ignore malformed */ }
    })

    server.addEventListener('close', () => {
      this.sessions.delete(server)
    })

    server.addEventListener('error', () => {
      this.sessions.delete(server)
    })

    return new Response(null, { status: 101, webSocket: client })
  }

  private broadcastToChannel(channelId: string, event: unknown, exclude?: WebSocket): void {
    const payload = JSON.stringify(event)
    for (const [ws, channelIds] of this.sessions) {
      if (ws === exclude) continue
      if (channelIds.has(channelId)) {
        try {
          ws.send(payload)
        } catch { /* stale connection */ }
      }
    }
  }
}
