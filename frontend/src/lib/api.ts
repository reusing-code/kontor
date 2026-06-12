const BASE = "/api/v1"
const TOKEN_KEY = "token"

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) {
    headers["Content-Type"] = "application/json"
  }
  const token = getToken()
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    clearToken()
    window.location.href = "/login"
    throw new ApiError(401, "unauthorized")
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new ApiError(res.status, data.error ?? res.statusText)
  }

  if (res.status === 204) return undefined as T
  return res.json()
}

export function get<T>(path: string): Promise<T> {
  return request<T>("GET", path)
}

export function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>("POST", path, body)
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return request<T>("PUT", path, body)
}

export function del(path: string): Promise<void> {
  return request<void>("DELETE", path)
}

export function delJson<T>(path: string): Promise<T> {
  return request<T>("DELETE", path)
}

export async function postForm<T>(path: string, body: FormData): Promise<T> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers,
    body,
  })

  if (res.status === 401) {
    clearToken()
    window.location.href = "/login"
    throw new ApiError(401, "unauthorized")
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new ApiError(res.status, data.error ?? res.statusText)
  }

  if (res.status === 204) return undefined as T
  return res.json()
}

export async function download(path: string, fallbackFilename: string): Promise<void> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  const res = await fetch(`${BASE}${path}`, { headers })

  if (res.status === 401) {
    clearToken()
    window.location.href = "/login"
    throw new ApiError(401, "unauthorized")
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new ApiError(res.status, data.error ?? res.statusText)
  }

  const disposition = res.headers.get("Content-Disposition") ?? ""
  const filename = /filename="([^"]+)"/.exec(disposition)?.[1] ?? fallbackFilename
  const url = URL.createObjectURL(await res.blob())
  const link = document.createElement("a")
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}
