import type { AuthConfig, AuthResponse, LoginData, RegisterData } from "@/types/auth"

const BASE = "/api/v1"

export async function login(data: LoginData): Promise<AuthResponse> {
  const res = await fetch(`${BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error ?? "Login failed")
  }
  return res.json()
}

export async function register(data: RegisterData): Promise<AuthResponse> {
  const res = await fetch(`${BASE}/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error ?? "Registration failed")
  }
  return res.json()
}

export async function getAuthConfig(): Promise<AuthConfig> {
  const res = await fetch(`${BASE}/auth/config`)
  if (!res.ok) {
    throw new Error("Could not load the login settings")
  }
  return res.json()
}
