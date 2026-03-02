import uuid
from contextlib import asynccontextmanager

from fastapi import FastAPI, Query, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from jose import JWTError
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request

from app.config import settings
from app.database import engine
from app.routers import auth, channels, messages, users, search, verification, tickets, admin, admissions, portfolio
from app.services import search_service
from app.utils.security import decode_token
from app.ws.manager import manager


@asynccontextmanager
async def lifespan(app: FastAPI):
    import asyncio
    loop = asyncio.get_event_loop()
    await loop.run_in_executor(None, search_service.setup_indexes)
    yield
    await engine.dispose()


app = FastAPI(
    title=settings.app_name,
    version="0.1.0",
    docs_url="/docs",
    redoc_url=None,
    lifespan=lifespan,
)

# ── CORS ──────────────────────────────────────────────────────
app.add_middleware(
    CORSMiddleware,
    allow_origins=[settings.frontend_url, "http://localhost:5173", "http://localhost:12003", *settings.extra_cors_origins],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ── Correlation-ID middleware ─────────────────────────────────
class CorrelationIdMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        cid = request.headers.get("X-Correlation-ID", str(uuid.uuid4()))
        request.state.correlation_id = cid
        response = await call_next(request)
        response.headers["X-Correlation-ID"] = cid
        return response


app.add_middleware(CorrelationIdMiddleware)

# ── Routers ───────────────────────────────────────────────────
app.include_router(auth.router, prefix="/api/v1/auth", tags=["auth"])
app.include_router(channels.router, prefix="/api/v1/channels", tags=["channels"])
app.include_router(messages.router, prefix="/api/v1/messages", tags=["messages"])
app.include_router(users.router, prefix="/api/v1/users", tags=["users"])
app.include_router(search.router, prefix="/api/v1/search", tags=["search"])
app.include_router(verification.router, prefix="/api/v1/verification", tags=["verification"])
app.include_router(tickets.router, prefix="/api/v1/tickets", tags=["tickets"])
app.include_router(admin.router, prefix="/api/v1/admin", tags=["admin"])
app.include_router(admissions.router, prefix="/api/v1/admissions", tags=["admissions"])
app.include_router(portfolio.router, prefix="/api/v1/portfolio", tags=["portfolio"])


@app.get("/health", tags=["health"])
async def health():
    return {"status": "ok"}


# ── WebSocket ─────────────────────────────────────────────────
@app.websocket("/ws")
async def websocket_endpoint(
    ws: WebSocket,
    token: str = Query(...),
):
    try:
        payload = decode_token(token, "access")
        user_id: str = payload["sub"]
    except JWTError:
        await ws.close(code=4001)
        return

    await manager.connect(ws)
    try:
        while True:
            data = await ws.receive_json()
            msg_type = data.get("type")
            channel_id = data.get("channelId")

            if msg_type == "subscribe" and channel_id:
                manager.subscribe(ws, channel_id)
            elif msg_type == "unsubscribe" and channel_id:
                manager.unsubscribe(ws, channel_id)
            elif msg_type == "typing" and channel_id:
                await manager.broadcast(
                    channel_id,
                    {"type": "typing", "data": {"userId": user_id, "channelId": channel_id}},
                    exclude_ws=ws,
                )
    except WebSocketDisconnect:
        manager.disconnect(ws)
