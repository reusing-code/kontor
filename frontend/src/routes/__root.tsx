import { Suspense, useEffect, useState } from "react"
import { createRootRoute, Outlet, Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { LanguageSwitcher } from "@/components/language-switcher"
import { Sidebar } from "@/components/sidebar"
import { useAuth } from "@/hooks/use-auth"
import { Button } from "@/components/ui/button"
import { LogOut } from "lucide-react"
import { useNavigate } from "@tanstack/react-router"

export const rootRoute = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  const { t } = useTranslation()
  const { isAuthenticated, logout } = useAuth()
  const matchRoute = useMatchRoute()
  const navigate = useNavigate()
  const isLoginPage = matchRoute({ to: "/login" })

  useEffect(() => {
    if (!isAuthenticated && !isLoginPage) {
      navigate({ to: "/login" })
    }
  }, [isAuthenticated, isLoginPage, navigate])

  if (isLoginPage) {
    return <Outlet />
  }

  if (!isAuthenticated) {
    return null
  }

  return (
    <div className="flex min-h-screen flex-col bg-background">
      <header className="border-b">
        <div className="flex h-14 items-center justify-between px-4">
          <Link to="/" className="text-lg font-semibold">
            {t("app.title")}
          </Link>
          <div className="flex items-center gap-2">
            <LanguageSwitcher />
            <Button variant="ghost" size="icon" onClick={logout} title={t("auth.logout")}>
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </header>
      <div className="flex flex-1">
        <Sidebar />
        <main className="flex-1 px-6 py-6">
          <Suspense fallback={<div className="text-sm text-muted-foreground">{t("common.loading")}</div>}>
            <Outlet />
          </Suspense>
        </main>
      </div>
      <VersionIndicator />
    </div>
  )
}

function VersionIndicator() {
  const [label, setLabel] = useState("")

  useEffect(() => {
    const controller = new AbortController()
    fetch("/api/version", { signal: controller.signal })
      .then((r) => {
        if (!r.ok) throw new Error(r.statusText)
        return r.json()
      })
      .then((data: { version: string; commit?: string; buildDate?: string }) => {
        const parts = [data.version]
        if (data.buildDate) parts.push(data.buildDate)
        if (data.commit) parts.push(data.commit)
        setLabel(parts.join(" / "))
      })
      .catch(() => {})
    return () => controller.abort()
  }, [])

  if (!label) return null

  return (
    <div className="py-1 text-center text-[11px] text-muted-foreground/60">
      <a href="https://github.com/reusing-code/contracts" target="_blank" rel="noopener noreferrer" className="hover:text-muted-foreground">
        Contracts
      </a>
      {" "}
      {label}
    </div>
  )
}
