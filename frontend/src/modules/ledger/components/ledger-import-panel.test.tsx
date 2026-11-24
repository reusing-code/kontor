import { describe, expect, it } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { usePendingDrafts } from "@/hooks/use-drafts"
import { LedgerImportPanel } from "@/modules/ledger/components/ledger-import-panel"
import { useLedgerAccounts } from "@/modules/ledger/hooks/use-ledger-accounts"
import { useLedgerReviewCount, useLedgerSuggestionStatus } from "@/modules/ledger/hooks/use-ledger-review"
import type { LedgerAccount, LedgerPreviewResult } from "@/modules/ledger/types"
import { apiError, mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { errorToasts } from "@/test/toasts"

const at = "2026-11-01T10:00:00Z"

const giro: LedgerAccount = { id: "acc-giro", name: "Giro", bank: "DKB", currency: "EUR", createdAt: at, updatedAt: at }
const savings: LedgerAccount = { id: "acc-savings", name: "Savings", bank: "DKB", currency: "EUR", createdAt: at, updatedAt: at }

function preview(overrides: Partial<LedgerPreviewResult> = {}): LedgerPreviewResult {
  return {
    previewId: "prev-1",
    sourceType: "dkb.csv",
    filename: "export.csv",
    fileSha256: "sha",
    iban: "DE02120300000000202051",
    bankName: "DKB",
    transactions: [
      {
        row: { bookingDate: "2026-10-30", amountMinor: -1250, currency: "EUR", counterpartyName: "Bakery Miller", purpose: "Bread" },
        fingerprint: "fp-1",
        isDuplicate: false,
        suggestedCategoryName: "Food",
        reviewStatus: "needsReview",
        categorizationSource: "keyword",
      },
      {
        row: { bookingDate: "2026-10-29", amountMinor: 250000, currency: "EUR", counterpartyName: "Employer", purpose: "Salary" },
        fingerprint: "fp-2",
        isDuplicate: true,
        reviewStatus: "confirmed",
        categorizationSource: "none",
      },
    ],
    totalRows: 2,
    newRows: 1,
    duplicateRows: 1,
    expiresAt: at,
    ...overrides,
  }
}

const committed = { batchId: "batch-1", accountId: "acc-giro", importedRows: 1, duplicateRows: 1 }

// The queries an import commit has to refresh, mounted as the ledger pages do.
function ImportedDataObserver() {
  useLedgerAccounts()
  useLedgerReviewCount()
  useLedgerSuggestionStatus(true)
  usePendingDrafts(true)
  return null
}

const observedGets = ["/ledger/accounts", "/ledger/transactions?limit=1", "/ledger/category-suggestions/status", "/drafts?status=pending"]

function mockLedger(routes: Record<string, unknown>) {
  return mockApi({
    "GET /ledger/accounts": [giro, savings],
    "GET /ledger/transactions": { items: [], total: 0 },
    "GET /ledger/category-suggestions/status": { running: false },
    "GET /drafts": [],
    ...routes,
  })
}

function renderPanel(accounts: LedgerAccount[]) {
  const view = renderWithProviders(
    <>
      <LedgerImportPanel accounts={accounts} />
      <ImportedDataObserver />
    </>,
  )
  const fileInput = view.container.querySelector<HTMLInputElement>('input[type="file"]')
  if (!fileInput) throw new Error("no file input")
  const csv = new File(["Buchungsdatum;Betrag\n"], "export.csv", { type: "text/csv" })
  const previewFile = async () => {
    await view.user.upload(fileInput, csv)
    await view.user.click(screen.getByRole("button", { name: "Preview" }))
  }
  return { ...view, previewFile, csv }
}

function getCounts(api: ReturnType<typeof mockApi>): number[] {
  return observedGets.map((path) => api.callsTo("GET", path).length)
}

async function selectOption(user: ReturnType<typeof renderPanel>["user"], placeholder: string, option: string) {
  const trigger = screen.getByText(placeholder).closest("button")
  if (!trigger) throw new Error(`no select ${placeholder}`)
  await user.click(trigger)
  await user.click(await screen.findByRole("option", { name: option }))
}

describe("LedgerImportPanel", () => {
  it("previews the uploaded file as multipart and shows the summary and rows", async () => {
    const api = mockLedger({ "POST /ledger/imports/preview": preview({ accountId: "acc-giro", warnings: ["Row 7 skipped"] }) })
    const { previewFile, csv } = renderPanel([giro, savings])

    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled()
    await previewFile()

    expect(await screen.findByText("Import preview ready.")).toBeInTheDocument()
    const [request] = api.callsTo("POST", "/ledger/imports/preview")
    expect(request.form?.get("file")).toBe(csv)
    expect(request.form?.get("sourceType")).toBe("dkb.csv")
    expect(request.form?.has("accountId")).toBe(false)

    expect(screen.getByText("Total rows").nextSibling).toHaveTextContent("2")
    expect(screen.getByText("New rows").nextSibling).toHaveTextContent("1")
    expect(screen.getByText("Duplicate rows").nextSibling).toHaveTextContent("1")
    expect(screen.getByText("Bakery Miller")).toBeInTheDocument()
    expect(screen.getByText("Employer")).toBeInTheDocument()
    expect(screen.getByText("Row 7 skipped")).toBeInTheDocument()
    // the resolved account is named, nothing is left to pick
    expect(screen.getByText("Giro")).toBeInTheDocument()
    expect(screen.queryByText("Resolve target account")).not.toBeInTheDocument()
  })

  it("sends the account picked for a comdirect file with the preview", async () => {
    const api = mockLedger({ "POST /ledger/imports/preview": preview({ sourceType: "comdirect.csv", accountId: "acc-savings" }) })
    const { user, previewFile } = renderPanel([giro, savings])

    await user.click(screen.getByText("DKB CSV").closest("button") as HTMLElement)
    await user.click(await screen.findByRole("option", { name: "comdirect CSV" }))
    await selectOption(user, "Select account for duplicate detection", "Savings")
    await previewFile()

    await screen.findByText("Import preview ready.")
    const [request] = api.callsTo("POST", "/ledger/imports/preview")
    expect(request.form?.get("sourceType")).toBe("comdirect.csv")
    expect(request.form?.get("accountId")).toBe("acc-savings")
  })

  it("commits into the resolved account, refreshes the imported data and resets", async () => {
    const api = mockLedger({
      "POST /ledger/imports/preview": preview({ accountId: "acc-giro" }),
      "POST /ledger/imports/prev-1/commit": committed,
    })
    const { user, previewFile } = renderPanel([giro, savings])
    await waitFor(() => expect(getCounts(api)).toEqual([1, 1, 1, 1]))

    await previewFile()
    await user.click(await screen.findByRole("button", { name: "Commit import" }))

    expect(await screen.findByText("Imported 1 transaction(s).")).toBeInTheDocument()
    expect(api.callsTo("POST", "/ledger/imports/prev-1/commit").map((c) => c.body)).toEqual([{ accountId: "acc-giro" }])
    await waitFor(() => expect(getCounts(api)).toEqual([2, 2, 2, 2]))
    expect(screen.queryByText("Total rows")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled()
  })

  it("commits into the existing account picked for an unknown IBAN", async () => {
    const api = mockLedger({
      "POST /ledger/imports/preview": preview(),
      "POST /ledger/imports/prev-1/commit": committed,
    })
    const { user, previewFile } = renderPanel([giro, savings])

    await previewFile()
    expect(await screen.findByText("Resolve target account")).toBeInTheDocument()
    const commit = screen.getByRole("button", { name: "Commit import" })
    expect(commit).toBeDisabled()

    await selectOption(user, "Select account", "Savings")
    await user.click(commit)

    await screen.findByText("Imported 1 transaction(s).")
    expect(api.callsTo("POST", "/ledger/imports/prev-1/commit").map((c) => c.body)).toEqual([{ accountId: "acc-savings" }])
  })

  it("commits into a new account prefilled with IBAN and bank of the file", async () => {
    const api = mockLedger({
      "POST /ledger/imports/preview": preview(),
      "POST /ledger/imports/prev-1/commit": committed,
    })
    const { user, previewFile } = renderPanel([giro])

    await previewFile()
    await user.click(await screen.findByRole("button", { name: "Create new account" }))
    expect(screen.getByLabelText("Bank")).toHaveValue("DKB")
    expect(screen.getByLabelText("IBAN")).toHaveValue("DE02120300000000202051")

    // the account needs a name first
    await user.click(screen.getByRole("button", { name: "Commit import" }))
    await waitFor(() => expect(screen.getByLabelText("Account name")).toHaveAttribute("aria-invalid", "true"))
    expect(api.callsTo("POST", "/ledger/imports/prev-1/commit")).toHaveLength(0)

    await user.type(screen.getByLabelText("Account name"), "Joint account")
    await user.click(screen.getByRole("button", { name: "Commit import" }))

    await screen.findByText("Imported 1 transaction(s).")
    expect(api.callsTo("POST", "/ledger/imports/prev-1/commit").map((c) => c.body)).toEqual([
      { newAccount: { name: "Joint account", bank: "DKB", iban: "DE02120300000000202051", currency: "EUR" } },
    ])
  })

  it("asks for a new account right away when there is none yet", async () => {
    mockLedger({ "POST /ledger/imports/preview": preview() })
    const { previewFile } = renderPanel([])

    await previewFile()

    expect(await screen.findByLabelText("Account name")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Use existing account" })).not.toBeInTheDocument()
  })

  it("shows one error toast for a failed preview and keeps the file for a retry", async () => {
    let attempts = 0
    const api = mockLedger({
      "POST /ledger/imports/preview": () => (++attempts === 1 ? apiError(400, "unknown CSV format") : preview({ accountId: "acc-giro" })),
    })
    const { user, previewFile } = renderPanel([giro])

    await previewFile()

    await waitFor(() => expect(errorToasts()).toEqual(["unknown CSV format"]))
    expect(screen.queryByText("Total rows")).not.toBeInTheDocument()
    const retry = screen.getByRole("button", { name: "Preview" })
    await waitFor(() => expect(retry).toBeEnabled())

    await user.click(retry)
    expect(await screen.findByText("Total rows")).toBeInTheDocument()
    expect(api.callsTo("POST", "/ledger/imports/preview")).toHaveLength(2)
    expect(errorToasts()).toEqual(["unknown CSV format"])
  })

  it("shows one error toast for a failed commit, keeps the preview and refreshes nothing", async () => {
    let attempts = 0
    const api = mockLedger({
      "POST /ledger/imports/preview": preview(),
      "POST /ledger/imports/prev-1/commit": (call: ApiCall) => (++attempts === 1 ? apiError(410, "preview expired") : { ...committed, accountId: (call.body as { accountId: string }).accountId }),
    })
    const { user, previewFile } = renderPanel([giro, savings])
    await waitFor(() => expect(getCounts(api)).toEqual([1, 1, 1, 1]))

    await previewFile()
    await selectOption(user, "Select account", "Savings")
    const commit = screen.getByRole("button", { name: "Commit import" })
    await user.click(commit)

    await waitFor(() => expect(errorToasts()).toEqual(["preview expired"]))
    expect(screen.getByText("Total rows")).toBeInTheDocument()
    expect(screen.getByText("Bakery Miller")).toBeInTheDocument()
    expect(getCounts(api)).toEqual([1, 1, 1, 1])

    await waitFor(() => expect(commit).toBeEnabled())
    await user.click(commit)
    await screen.findByText("Imported 1 transaction(s).")
    expect(api.callsTo("POST", "/ledger/imports/prev-1/commit").map((c) => c.body)).toEqual([
      { accountId: "acc-savings" },
      { accountId: "acc-savings" },
    ])
    expect(errorToasts()).toEqual(["preview expired"])
  })

  it("drops the preview on Cancel without committing", async () => {
    const api = mockLedger({ "POST /ledger/imports/preview": preview() })
    const { user, previewFile } = renderPanel([giro, savings])

    await previewFile()
    await user.click(await screen.findByRole("button", { name: "Create new account" }))
    await user.click(screen.getByRole("button", { name: "Cancel" }))

    expect(screen.queryByText("Total rows")).not.toBeInTheDocument()
    expect(screen.queryByLabelText("Account name")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled()
    expect(api.calls.filter((c) => c.method === "POST" && c.path.endsWith("/commit"))).toHaveLength(0)
  })

  it("clears the file input on Cancel so the same file can be picked again", async () => {
    mockLedger({ "POST /ledger/imports/preview": preview({ accountId: "acc-giro" }) })
    const { user, container, previewFile } = renderPanel([giro])

    await previewFile()
    await user.click(await screen.findByRole("button", { name: "Cancel" }))

    expect(container.querySelector<HTMLInputElement>('input[type="file"]')?.files).toHaveLength(0)
  })
})
