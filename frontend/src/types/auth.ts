export interface LoginData {
  email: string
  password: string
}

export interface RegisterData {
  email: string
  password: string
}

export interface AuthUser {
  id: string
  email: string
  createdAt: string
}

export interface AuthResponse {
  token: string
  user: AuthUser
}

export interface AuthConfig {
  registrationOpen: boolean
}
