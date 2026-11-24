import { describe, expect, it } from "vitest"
import { act, screen, within } from "@testing-library/react"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerAccount } from "@/modules/ledger/types"
import { LedgerSidebarSection } from "@/modules/ledger/components/sidebar-section"

const at = "2026-11-01T10:00:00Z"

function account(id: string, name: string): LedgerAccount {
  return { id, name, bank: "DKB", currency: "EUR", createdAt: at, updatedAt: at } as LedgerAccount
}

const paths = [
  "/ledger",
  "/ledger/transactions",
  "/ledger/review",
  "/ledger/analysis",
  "/ledger/email-orders",
  "/ledger/accounts/$accountId",
  "/ledger/categories",
  "/ledger/bank-connections",
  "/ledger/email-accounts",
]

async function renderAt(path: string, { bankImport = true, accounts = [account("acc-1", "Giro"), account("acc-2", "Savings")] } = {}) {
  mockApi({
    "GET /capabilities": { llm: false, llmVision: false, bankImport },
    "GET /ledger/accounts": accounts,
  })
  const rootRoute = createRootRoute({
    component: () => (
      <>
        <nav aria-label="sidebar"><LedgerSidebarSection /></nav>
        <Outlet />
      </>
    ),
  })
  const routeTree = rootRoute.addChildren(paths.map((p) => createRoute({ getParentRoute: () => rootRoute, path: p, component: () => null })))
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) })
  const view = renderWithProviders(<RouterProvider router={router} />)
  await screen.findByRole("link", { name: "Ledger" })
  return { ...view, router }
}

function linkNames(): string[] {
  return within(screen.getByRole("navigation", { name: "sidebar" })).getAllByRole("link").map((link) => link.textContent ?? "")
}

describe("LedgerSidebarSection", () => {
  it("lists the data pages first, then the collapsed accounts and the administration", async () => {
    await renderAt("/ledger")

    await screen.findByRole("link", { name: "Bank connections" })
    expect(linkNames()).toEqual(["Ledger", "Transactions", "Review queue", "Analysis", "Email orders", "Categories", "Bank connections", "Email accounts"])
    expect(screen.getByRole("button", { name: "Accounts" })).toHaveAttribute("aria-expanded", "false")
    expect(screen.getByRole("button", { name: "Administration" })).toHaveAttribute("aria-expanded", "true")
  })

  it("shows the accounts when the sub-section is opened", async () => {
    const { user } = await renderAt("/ledger")

    await user.click(await screen.findByRole("button", { name: "Accounts" }))
    expect(screen.getByRole("link", { name: "Giro" })).toHaveAttribute("href", "/ledger/accounts/acc-1")
    expect(screen.getByRole("link", { name: "Savings" })).toBeInTheDocument()
    expect(linkNames().indexOf("Giro")).toBeLessThan(linkNames().indexOf("Categories"))
  })

  it("opens the accounts on an account page and marks the account", async () => {
    await renderAt("/ledger/accounts/acc-2")

    const savings = await screen.findByRole("link", { name: "Savings" })
    expect(screen.getByRole("button", { name: "Accounts" })).toHaveAttribute("aria-expanded", "true")
    expect(savings).toHaveClass("bg-accent")
    expect(screen.getByRole("link", { name: "Giro" })).not.toHaveClass("bg-accent")
  })

  it("hides the bank connections without the bankImport capability and the accounts without accounts", async () => {
    await renderAt("/ledger", { bankImport: false, accounts: [] })

    await screen.findByRole("link", { name: "Categories" })
    expect(screen.queryByRole("link", { name: "Bank connections" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Accounts" })).toBeNull()
  })

  it("opens a collapsed administration again when one of its pages becomes active", async () => {
    const { user, router } = await renderAt("/ledger")

    await user.click(await screen.findByRole("button", { name: "Administration" }))
    expect(screen.queryByRole("link", { name: "Categories" })).toBeNull()
    await act(() => router.navigate({ to: "/ledger/categories" }))
    expect(screen.getByRole("button", { name: "Administration" })).toHaveAttribute("aria-expanded", "true")
    expect(screen.getByRole("link", { name: "Categories" })).toHaveClass("bg-accent")
  })
})
