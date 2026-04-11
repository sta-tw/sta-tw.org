import { SignJWT, jwtVerify } from 'jose'

export async function createAccessToken(userId: string, secret: string, expireMinutes: number): Promise<string> {
  const key = new TextEncoder().encode(secret)
  return new SignJWT({ sub: userId, type: 'access' })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime(`${expireMinutes}m`)
    .sign(key)
}

export async function createRefreshToken(userId: string, sessionId: string, secret: string, expireDays: number): Promise<string> {
  const key = new TextEncoder().encode(secret)
  return new SignJWT({ sub: userId, jti: sessionId, type: 'refresh' })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime(`${expireDays}d`)
    .sign(key)
}

export async function createEmailToken(userId: string, tokenType: string, secret: string, hours = 24): Promise<string> {
  const key = new TextEncoder().encode(secret)
  return new SignJWT({ sub: userId, type: tokenType })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime(`${hours}h`)
    .sign(key)
}

export async function decodeToken(token: string, expectedType: string, secret: string): Promise<Record<string, unknown>> {
  const key = new TextEncoder().encode(secret)
  const { payload } = await jwtVerify(token, key)
  if (payload.type !== expectedType) {
    throw new Error(`Expected token type '${expectedType}', got '${payload.type}'`)
  }
  return payload as Record<string, unknown>
}
