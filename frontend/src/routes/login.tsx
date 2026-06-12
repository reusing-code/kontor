import { useState } from "react"
import { createRoute, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { Eye, EyeOff } from "lucide-react"
import { rootRoute } from "./__root"
import { useAuth } from "@/hooks/use-auth"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
})

function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
  autoComplete,
}: {
  id: string
  value: string
  onChange: (value: string) => void
  placeholder: string
  autoComplete: string
}) {
  const [visible, setVisible] = useState(false)

  return (
    <div className="relative">
      <Input
        id={id}
        type={visible ? "text" : "password"}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required
        autoComplete={autoComplete}
        className="pr-10"
      />
      <button
        type="button"
        tabIndex={-1}
        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
        onClick={() => setVisible(!visible)}
      >
        {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

function LoginPage() {
  const { t } = useTranslation()
  const { login, register } = useAuth()
  const navigate = useNavigate()

  const [isRegister, setIsRegister] = useState(false)
  usePageTitle(isRegister ? t("auth.register") : t("auth.login"), t("app.title"))
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError("")

    if (isRegister && password.length < 8) {
      setError(t("auth.passwordTooShort"))
      return
    }

    if (isRegister && password !== confirmPassword) {
      setError(t("auth.passwordMismatch"))
      return
    }

    setLoading(true)

    try {
      if (isRegister) {
        await register({ email, password })
      } else {
        await login({ email, password })
      }
      navigate({ to: "/" })
    } catch (err) {
      setError(err instanceof Error ? err.message : "An error occurred")
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-center text-xl">
            {isRegister ? t("auth.registerTitle") : t("auth.loginTitle")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">{t("auth.email")}</Label>
              <Input
                id="email"
                type="email"
                placeholder={t("auth.emailPlaceholder")}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoComplete="email"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t("auth.password")}</Label>
              <PasswordInput
                id="password"
                placeholder={t("auth.passwordPlaceholder")}
                value={password}
                onChange={setPassword}
                autoComplete={isRegister ? "new-password" : "current-password"}
              />
            </div>

            {isRegister && (
              <div className="space-y-2">
                <Label htmlFor="confirmPassword">{t("auth.confirmPassword")}</Label>
                <PasswordInput
                  id="confirmPassword"
                  placeholder={t("auth.confirmPasswordPlaceholder")}
                  value={confirmPassword}
                  onChange={setConfirmPassword}
                  autoComplete="new-password"
                />
              </div>
            )}

            {error && (
              <p className="text-sm text-destructive">{error}</p>
            )}

            <Button type="submit" className="w-full" disabled={loading}>
              {isRegister ? t("auth.register") : t("auth.login")}
            </Button>
          </form>

          <button
            type="button"
            className="mt-4 w-full text-center text-sm text-muted-foreground hover:underline"
            onClick={() => {
              setIsRegister(!isRegister)
              setError("")
              setConfirmPassword("")
            }}
          >
            {isRegister ? t("auth.switchToLogin") : t("auth.switchToRegister")}
          </button>
        </CardContent>
      </Card>
    </div>
  )
}
