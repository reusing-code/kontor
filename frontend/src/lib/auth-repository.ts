import type { AuthConfig, AuthResponse, LoginData, RegisterData } from "@/types/auth"
import { sendJson } from "@/lib/api"

export function login(data: LoginData): Promise<AuthResponse> {
  return sendJson<AuthResponse>("POST", "/auth/login", { json: data, redirectOnUnauthorized: false })
}

export function register(data: RegisterData): Promise<AuthResponse> {
  return sendJson<AuthResponse>("POST", "/auth/register", { json: data, redirectOnUnauthorized: false })
}

export function getAuthConfig(): Promise<AuthConfig> {
  return sendJson<AuthConfig>("GET", "/auth/config", { redirectOnUnauthorized: false })
}
