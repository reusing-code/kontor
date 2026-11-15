import { describe, expect, it } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { InboxPage } from "@/routes/inbox"
import { apiError, mockApi } from "@/test/api-mock"
import { acceptCases, draft, ids, inboxRoutes } from "@/test/inbox-fixtures"
import { renderWithProviders } from "@/test/render"
import type { Draft } from "@/types/drafts"

// These tests mount the real module dialogs; prefilled-create-dialogs.test.tsx
// checks the exact initialValues with stubbed dialogs.

function card(d: Draft): HTMLElement {
  return screen.getByText(d.source.title!).closest<HTMLElement>('[data-slot="card"]')!
}

async function findAccept(d: Draft) {
  await screen.findByText(d.source.title!)
  return within(card(d)).getByRole("button", { name: "Accept…" })
}

describe("InboxPage accept", () => {
  it.each(acceptCases)("opens the $type dialog prefilled, creates the entry and accepts the draft", async (c) => {
    const d = draft(c.type, c.input, { parentId: c.parentId })
    const api = mockApi(inboxRoutes({ drafts: [d] }))
    const { user } = renderWithProviders(<InboxPage />)

    const accept = await findAccept(d)
    await waitFor(() => expect(accept).toBeEnabled())
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
    await user.click(accept)

    const dialog = await screen.findByRole("dialog", { name: c.dialogTitle })
    expect(screen.getAllByRole("dialog")).toHaveLength(1)
    for (const value of c.shown) {
      expect(within(dialog).getByDisplayValue(value)).toBeInTheDocument()
    }
    expect(within(dialog).queryByDisplayValue("dropped")).not.toBeInTheDocument()

    await user.click(within(dialog).getByRole("button", { name: "Save" }))

    await waitFor(() => expect(api.callsTo("POST", `/drafts/${d.id}/accept`)).toHaveLength(1))
    const creates = api.callsTo("POST", c.createPath)
    expect(creates).toHaveLength(1)
    expect(creates[0].body).toMatchObject(c.created)
    expect(creates[0].body).not.toHaveProperty("unknownField")
    expect(api.callsTo("POST", `/drafts/${d.id}/accept`)[0].body).toEqual({
      entityType: c.type,
      entityId: ids.created,
      entityUrl: window.location.origin + c.entityPath,
    })
    // The create must come first: the draft is only accepted with an existing entity.
    const order = api.calls.filter((call) => call.method === "POST").map((call) => call.path)
    expect(order).toEqual([c.createPath, `/drafts/${d.id}/accept`])

    await waitFor(() => expect(screen.queryByText(d.source.title!)).not.toBeInTheDocument())
    expect(await screen.findByText("Draft accepted and document linked")).toBeInTheDocument()
    expect(await screen.findByText("No drafts waiting for review.")).toBeInTheDocument()
  })

  it("does not accept the draft when creating the entry fails", async () => {
    const d = draft("vehicle", { name: "Family car" })
    const api = mockApi({ ...inboxRoutes({ drafts: [d] }), "POST /vehicles": apiError(400, "name taken") })
    const { user } = renderWithProviders(<InboxPage />)

    await user.click(await findAccept(d))
    const dialog = await screen.findByRole("dialog", { name: "Add Vehicle" })
    await user.click(within(dialog).getByRole("button", { name: "Save" }))

    expect(await screen.findByText("Could not create the entry: name taken")).toBeInTheDocument()
    expect(api.callsTo("POST", `/drafts/${d.id}/accept`)).toHaveLength(0)
    expect(screen.getByText(d.source.title!)).toBeInTheDocument()
  })

  it("closes the dialog without creating anything on cancel", async () => {
    const d = draft("ledger_account", { name: "Giro" })
    const api = mockApi(inboxRoutes({ drafts: [d] }))
    const { user } = renderWithProviders(<InboxPage />)

    await user.click(await findAccept(d))
    const dialog = await screen.findByRole("dialog", { name: "Create account" })
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
    expect(api.calls.filter((call) => call.method === "POST")).toHaveLength(0)
    expect(screen.getByText(d.source.title!)).toBeInTheDocument()
  })
})

describe("InboxPage accept availability", () => {
  it("keeps drafts of a disabled module visible with Accept disabled", async () => {
    const cost = draft("cost", { type: "fuel", vendor: "Shell" }, { parentId: ids.vehicle })
    const vehicle = draft("vehicle", { name: "Family car" })
    const purchase = draft("purchase", { itemName: "Standing desk" }, { parentId: ids.purchaseCategory })
    mockApi(inboxRoutes({ drafts: [cost, vehicle, purchase], enabledModules: ["contracts", "purchases", "ledger"] }))
    const { user } = renderWithProviders(<InboxPage />)

    const purchaseAccept = await findAccept(purchase)
    await waitFor(() => expect(purchaseAccept).toBeEnabled())
    expect(within(card(purchase)).queryByText(/This module is disabled/)).not.toBeInTheDocument()

    for (const d of [cost, vehicle]) {
      expect(within(card(d)).getByRole("button", { name: "Accept…" })).toBeDisabled()
      expect(within(card(d)).getByText("This module is disabled. Choose another type or reject the draft.")).toBeInTheDocument()
      expect(within(card(d)).getByRole("button", { name: "Reject" })).toBeEnabled()
    }

    await user.click(within(card(cost)).getByRole("button", { name: "Accept…" }))
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()

    // The type picker offers the disabled module only for the draft that has it.
    await user.click(within(card(purchase)).getByRole("combobox", { name: "Type" }))
    const offered = (await screen.findAllByRole("option")).map((o) => o.textContent)
    expect(offered).toHaveLength(7)
    expect(offered).toContain("Unclassified")
  })

  it("enables Accept for every type while all modules are enabled", async () => {
    const drafts = acceptCases.map((c) => draft(c.type, c.input, { parentId: c.parentId }))
    mockApi(inboxRoutes({ drafts }))
    renderWithProviders(<InboxPage />)

    for (const d of drafts) {
      const accept = await findAccept(d)
      await waitFor(() => expect(accept).toBeEnabled())
    }
  })

  it("disables Accept while the type or the required parent is missing", async () => {
    const unclassified = draft("none", {})
    const noCategory = draft("contract", { name: "Fiber 500" })
    const unknownVehicle = draft("cost", { type: "fuel" }, { parentId: "gone" })
    const emptyModule = draft("purchase", { itemName: "Standing desk" }, { parentId: ids.purchaseCategory })
    const optionalParent = draft("ledger_category", { name: "Groceries" })
    mockApi(inboxRoutes({ drafts: [unclassified, noCategory, unknownVehicle, emptyModule, optionalParent], purchaseCategories: false }))
    renderWithProviders(<InboxPage />)

    const optionalAccept = await findAccept(optionalParent)
    await waitFor(() => expect(optionalAccept).toBeEnabled())
    const hints: [Draft, string][] = [
      [unclassified, "Choose what this document is, or reject it."],
      [noCategory, "Choose a category to accept the draft."],
      [unknownVehicle, "Choose a vehicle to accept the draft."],
      [emptyModule, "Create a category in this module first."],
    ]
    for (const [d, hint] of hints) {
      await within(card(d)).findByText(hint)
      expect(within(card(d)).getByRole("button", { name: "Accept…" })).toBeDisabled()
    }
  })
})

describe("InboxPage list", () => {
  it("shows the empty state without drafts", async () => {
    mockApi(inboxRoutes({ drafts: [] }))
    renderWithProviders(<InboxPage />)

    expect(await screen.findByText("No drafts waiting for review.")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Accept…" })).not.toBeInTheDocument()
  })

  it("rejects a draft", async () => {
    const d = draft("vehicle", { name: "Family car" })
    const api = mockApi(inboxRoutes({ drafts: [d] }))
    const { user } = renderWithProviders(<InboxPage />)

    await findAccept(d)
    await user.click(within(card(d)).getByRole("button", { name: "Reject" }))

    expect(await screen.findByText("Draft rejected")).toBeInTheDocument()
    expect(api.callsTo("POST", `/drafts/${d.id}/reject`)).toHaveLength(1)
    await waitFor(() => expect(screen.queryByText(d.source.title!)).not.toBeInTheDocument())
  })
})
