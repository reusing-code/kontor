import { beforeEach, describe, expect, it, vi } from "vitest"
import { render, screen } from "@testing-library/react"
import { QueryClientProvider } from "@tanstack/react-query"
import { PrefilledCreateDialogs } from "@/components/drafts/prefilled-create-dialogs"
import { draftEntityTypes, type PrefillEntry } from "@/components/drafts/draft-input"
import { mockApi } from "@/test/api-mock"
import { acceptCases, ids } from "@/test/inbox-fixtures"
import { createTestQueryClient, renderWithProviders } from "@/test/render"

interface StubProps {
  open: boolean
  initialValues?: Record<string, unknown>
  initialParentId?: string
  onSubmit: (data: Record<string, unknown>) => unknown
}

// Every render of a stubbed dialog, by dialog name, and what the last submit returned.
const renders = vi.hoisted(() => ({ list: [] as { name: string; props: StubProps }[], submitted: undefined as unknown }))

function stub(name: string) {
  return (props: StubProps) => {
    renders.list.push({ name, props })
    return props.open ? (
      <button data-testid="open-dialog" onClick={() => (renders.submitted = props.onSubmit({ submittedBy: name }))}>
        {name}
      </button>
    ) : null
  }
}

vi.mock("@/modules/purchases/components/purchase-dialog", () => ({ PurchaseDialog: stub("PurchaseDialog") }))
vi.mock("@/modules/contracts/components/contract-dialog", () => ({ ContractDialog: stub("ContractDialog") }))
vi.mock("@/modules/auto/components/cost-entry-dialog", () => ({ CostEntryDialog: stub("CostEntryDialog") }))
vi.mock("@/modules/auto/components/vehicle-dialog", () => ({ VehicleDialog: stub("VehicleDialog") }))
vi.mock("@/components/category-dialog", () => ({ CategoryDialog: stub("CategoryDialog") }))
vi.mock("@/modules/ledger/components/ledger-category-dialog", () => ({ LedgerCategoryDialog: stub("LedgerCategoryDialog") }))
vi.mock("@/modules/ledger/components/ledger-account-dialog", () => ({ LedgerAccountDialog: stub("LedgerAccountDialog") }))

const dialogFor: Record<PrefillEntry["type"], string> = {
  purchase: "PurchaseDialog",
  contract: "ContractDialog",
  cost: "CostEntryDialog",
  vehicle: "VehicleDialog",
  contract_category: "CategoryDialog",
  purchase_category: "CategoryDialog",
  ledger_category: "LedgerCategoryDialog",
  ledger_account: "LedgerAccountDialog",
}

// The last render of each of the eight dialog instances, in mount order.
function lastRenders() {
  return renders.list.slice(-8)
}

function serverRoutes() {
  return {
    "GET /ledger/categories": [],
    ...Object.fromEntries(acceptCases.map((c) => [`POST ${c.createPath}`, () => ({ id: ids.created })])),
  }
}

const ledgerTypes: PrefillEntry["type"][] = ["ledger_category", "ledger_account"]

beforeEach(() => {
  renders.list = []
  renders.submitted = undefined
})

describe("PrefilledCreateDialogs", () => {
  it("covers every draft entity type", () => {
    expect(acceptCases.map((c) => c.type)).toEqual(draftEntityTypes)
  })

  it("opens no dialog without an entry", () => {
    mockApi(serverRoutes())
    renderWithProviders(<PrefilledCreateDialogs entry={null} onClose={() => {}} onCreate={() => Promise.resolve()} />)

    expect(screen.queryByTestId("open-dialog")).not.toBeInTheDocument()
    expect(lastRenders()).toHaveLength(8)
    for (const { props } of lastRenders()) {
      expect(props.open).toBe(false)
      expect(props.initialValues).toBeUndefined()
    }
  })

  it.each(acceptCases)("opens only the $type dialog with its initialValues", (c) => {
    mockApi(serverRoutes())
    const entry: PrefillEntry = { type: c.type, parentId: c.parentId ?? "", input: c.input }
    renderWithProviders(<PrefilledCreateDialogs entry={entry} onClose={() => {}} onCreate={() => Promise.resolve()} />)

    expect(screen.getAllByTestId("open-dialog").map((el) => el.textContent)).toEqual([dialogFor[c.type]])
    const open = lastRenders().filter((r) => r.props.open)
    expect(open).toHaveLength(1)
    expect(open[0].name).toBe(dialogFor[c.type])
    expect(open[0].props.initialValues).toEqual(c.initialValues)
    for (const { props } of lastRenders().filter((r) => !r.props.open)) {
      expect(props.initialValues).toBeUndefined()
    }
    const ledgerCategory = lastRenders().find((r) => r.name === "LedgerCategoryDialog")!
    expect(ledgerCategory.props.initialParentId).toBe(c.type === "ledger_category" ? c.parentId : undefined)
  })

  it.each(acceptCases)("creates the $type through its module endpoint on submit", async (c) => {
    const api = mockApi(serverRoutes())
    const entry: PrefillEntry = { type: c.type, parentId: c.parentId ?? "", input: c.input }
    const onCreate = vi.fn<(entry: PrefillEntry, create: () => Promise<{ id: string }>) => Promise<unknown>>(() => Promise.resolve())
    const { user } = renderWithProviders(<PrefilledCreateDialogs entry={entry} onClose={() => {}} onCreate={onCreate} />)

    await user.click(screen.getByTestId("open-dialog"))

    expect(onCreate).toHaveBeenCalledTimes(1)
    expect(onCreate.mock.calls[0][0]).toBe(entry)
    expect(api.calls.filter((call) => call.method === "POST")).toHaveLength(0)
    await expect(onCreate.mock.calls[0][1]()).resolves.toEqual({ id: ids.created })
    expect(api.calls.filter((call) => call.method === "POST")).toEqual([
      { method: "POST", path: c.createPath, body: { submittedBy: dialogFor[c.type] } },
    ])
  })

  // The module dialogs close themselves once their onSubmit resolves; the
  // ledger dialogs do not, so PrefilledCreateDialogs closes them.
  it.each(acceptCases)("passes a failed $type create back to the dialog and closes only after success", async (c) => {
    mockApi(serverRoutes())
    const entry: PrefillEntry = { type: c.type, parentId: c.parentId ?? "", input: c.input }
    const onClose = vi.fn()
    const onCreate = vi
      .fn<(entry: PrefillEntry, create: () => Promise<{ id: string }>) => Promise<unknown>>()
      .mockRejectedValueOnce(new Error("name taken"))
      .mockResolvedValueOnce({ id: ids.created })
    const { user } = renderWithProviders(<PrefilledCreateDialogs entry={entry} onClose={onClose} onCreate={onCreate} />)
    const ledger = ledgerTypes.includes(c.type)

    await user.click(screen.getByTestId("open-dialog"))
    if (!ledger) await expect(renders.submitted).rejects.toThrow("name taken")
    expect(onClose).not.toHaveBeenCalled()

    await user.click(screen.getByTestId("open-dialog"))
    if (!ledger) await expect(renders.submitted).resolves.toEqual({ id: ids.created })
    await vi.waitFor(() => expect(onClose).toHaveBeenCalledTimes(ledger ? 1 : 0))
    expect(onCreate).toHaveBeenCalledTimes(2)
  })

  it("keeps the initialValues object stable across rerenders", () => {
    mockApi(serverRoutes())
    const c = acceptCases[0]
    const entry: PrefillEntry = { type: c.type, parentId: c.parentId ?? "", input: c.input }
    const queryClient = createTestQueryClient()
    const ui = () => (
      <QueryClientProvider client={queryClient}>
        <PrefilledCreateDialogs entry={entry} onClose={() => {}} onCreate={() => Promise.resolve()} />
      </QueryClientProvider>
    )
    const { rerender } = render(ui())
    const first = lastRenders().find((r) => r.props.open)!.props.initialValues
    rerender(ui())

    expect(lastRenders().find((r) => r.props.open)!.props.initialValues).toBe(first)
  })
})
