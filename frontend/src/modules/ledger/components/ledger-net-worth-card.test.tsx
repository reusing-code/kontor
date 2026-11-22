import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import type { LedgerAccount } from "@/modules/ledger/types"
import { LedgerNetWorthCard } from "@/modules/ledger/components/ledger-net-worth-card"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}))

const at = "2026-11-01T10:00:00Z"

function account(name: string, currency: string, currentMinor?: number): LedgerAccount {
  return {
    id: `acc-${name}`,
    name,
    bank: "DKB",
    currency,
    createdAt: at,
    updatedAt: at,
    balance: currentMinor === undefined ? undefined : { currentMinor, snapshotMinor: currentMinor, asOf: "2026-11-01", source: "dkb.csv" },
  }
}

const analysisLink = () => screen.getByRole("link", { name: "Open analysis" })

describe("LedgerNetWorthCard", () => {
  it("shows the sum of the balances and links to the analysis", () => {
    renderWithProviders(<LedgerNetWorthCard accounts={[account("Giro", "EUR", 150000), account("Savings", "EUR", 1000050)]} />)

    expect(screen.getByText("Net worth")).toBeInTheDocument()
    expect(screen.getByText("€11,500.50")).toBeInTheDocument()
    expect(screen.getByText("Sum of the balances of 2 accounts")).toBeInTheDocument()
    expect(screen.queryByText(/not included/)).toBeNull()
    expect(analysisLink()).toHaveAttribute("href", "/ledger/analysis")
  })

  it("says how many accounts are not included", () => {
    const { unmount } = renderWithProviders(
      <LedgerNetWorthCard accounts={[account("Giro", "EUR", -2500), account("Dollars", "USD", 999999), account("Cash", "EUR")]} />,
    )
    expect(screen.getByText("-€25.00")).toBeInTheDocument()
    expect(screen.getByText("Balance of 1 account")).toBeInTheDocument()
    expect(screen.getByText("2 accounts are not included.")).toBeInTheDocument()
    unmount()

    renderWithProviders(<LedgerNetWorthCard accounts={[account("Giro", "EUR", 100), account("Cash", "EUR")]} />)
    expect(screen.getByText("1 account is not included.")).toBeInTheDocument()
  })

  it("shows a placeholder instead of an amount without any balance", () => {
    const { unmount } = renderWithProviders(<LedgerNetWorthCard accounts={[account("Cash", "EUR"), account("Dollars", "USD", 5000)]} />)
    expect(screen.getByText("–")).toBeInTheDocument()
    expect(screen.getByText("No account has a balance yet.")).toBeInTheDocument()
    expect(screen.getByText("2 accounts are not included.")).toBeInTheDocument()
    expect(screen.queryByText(/€/)).toBeNull()
    expect(analysisLink()).toHaveAttribute("href", "/ledger/analysis")
    unmount()

    renderWithProviders(<LedgerNetWorthCard accounts={[]} />)
    expect(screen.getByText("–")).toBeInTheDocument()
    expect(screen.queryByText(/not included/)).toBeNull()
    expect(analysisLink()).toHaveAttribute("href", "/ledger/analysis")
  })

  it("says nothing about the accounts while they are loading", () => {
    renderWithProviders(<LedgerNetWorthCard accounts={[]} isPending />)

    expect(screen.getByText("–")).toBeInTheDocument()
    expect(screen.queryByText("No account has a balance yet.")).toBeNull()
    expect(analysisLink()).toHaveAttribute("href", "/ledger/analysis")
  })
})
