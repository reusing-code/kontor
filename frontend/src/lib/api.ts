const BASE = "/api/v1"
const TOKEN_KEY = "token"
export const USER_KEY = "user"

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export function clearSession(): void {
  clearToken()
  localStorage.removeItem(USER_KEY)
}

export class ApiError extends Error {
  status: number
  // Stable machine-readable code of the endpoint, when it sends one.
  code?: string
  // The parsed error body, for endpoints that send fields beyond error/code.
  body: Record<string, unknown>

  constructor(status: number, message: string, code?: string, body: Record<string, unknown> = {}) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
    this.body = body
  }
}

export interface SendOptions {
  json?: unknown
  form?: FormData
  // false: a 401 is an ordinary error (wrong credentials on login, register
  // and the password change) instead of an expired session.
  redirectOnUnauthorized?: boolean
}

// send is the one place that talks to the API: auth header, body encoding,
// the expired-session redirect and ApiError for every non-2xx answer.
export async function send(method: string, path: string, options: SendOptions = {}): Promise<Response> {
  const { json, form, redirectOnUnauthorized = true } = options
  const headers: Record<string, string> = {}
  if (json !== undefined) {
    headers["Content-Type"] = "application/json"
  }
  const token = getToken()
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: form ?? (json !== undefined ? JSON.stringify(json) : undefined),
  })

  if (res.status === 401 && redirectOnUnauthorized) {
    clearSession()
    window.location.href = "/login"
    throw new ApiError(401, "unauthorized")
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new ApiError(res.status, data.error ?? res.statusText, data.code, data)
  }

  return res
}

export async function sendJson<T>(method: string, path: string, options?: SendOptions): Promise<T> {
  const res = await send(method, path, options)
  if (res.status === 204) return undefined as T
  return res.json()
}

export function get<T>(path: string): Promise<T> {
  return sendJson<T>("GET", path)
}

export function post<T>(path: string, body: unknown): Promise<T> {
  return sendJson<T>("POST", path, { json: body })
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return sendJson<T>("PUT", path, { json: body })
}

export async function del(path: string): Promise<void> {
  await send("DELETE", path)
}

export function delJson<T>(path: string): Promise<T> {
  return sendJson<T>("DELETE", path)
}

export function postForm<T>(path: string, body: FormData): Promise<T> {
  return sendJson<T>("POST", path, { form: body })
}

export async function getBlob(path: string): Promise<Blob> {
  const res = await send("GET", path)
  return res.blob()
}

export async function download(path: string, fallbackFilename: string): Promise<void> {
  const res = await send("GET", path)
  const disposition = res.headers.get("Content-Disposition") ?? ""
  const filename = /filename="([^"]+)"/.exec(disposition)?.[1] ?? fallbackFilename
  const url = URL.createObjectURL(await res.blob())
  const link = document.createElement("a")
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}
