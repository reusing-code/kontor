import { createContext, useContext, useState, useCallback } from "react"
import type { ReactNode } from "react"
import { USER_KEY, clearSession, getToken, setToken } from "@/lib/api"
import { login as apiLogin, register as apiRegister } from "@/lib/auth-repository"
import type { AuthUser, LoginData, RegisterData } from "@/types/auth"

function loadUser(): AuthUser | null {
  try {
    const raw = localStorage.getItem(USER_KEY)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
  }
}

interface AuthContextValue {
  user: AuthUser | null
  token: string | null
  login: (data: LoginData) => Promise<void>
  register: (data: RegisterData) => Promise<void>
  logout: () => void
  isAuthenticated: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(loadUser)
  const [token, setTokenState] = useState<string | null>(getToken)

  const login = useCallback(async (data: LoginData) => {
    const res = await apiLogin(data)
    setToken(res.token)
    localStorage.setItem(USER_KEY, JSON.stringify(res.user))
    setTokenState(res.token)
    setUser(res.user)
  }, [])

  const register = useCallback(async (data: RegisterData) => {
    const res = await apiRegister(data)
    setToken(res.token)
    localStorage.setItem(USER_KEY, JSON.stringify(res.user))
    setTokenState(res.token)
    setUser(res.user)
  }, [])

  const logout = useCallback(() => {
    clearSession()
    setTokenState(null)
    setUser(null)
    window.location.href = "/login"
  }, [])

  return (
    <AuthContext.Provider value={{
      user,
      token,
      login,
      register,
      logout,
      isAuthenticated: !!token,
    }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error("useAuth must be used within AuthProvider")
  }
  return ctx
}
