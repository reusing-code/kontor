import { describe, expect, it } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { InboxPage } from "@/routes/inbox"
import { apiError, mockApi } from "@/test/api-mock"
import { acceptCases, draft, ids, inboxRoutes } from "@/test/inbox-fixtures"
import { renderWithProviders } from "@/test/render"
import { errorToasts } from "@/test/toasts"
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
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
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

  it.each(acceptCases)("keeps the $type dialog open with the typed input while the create fails", async (c) => {
    const d = draft(c.type, c.input, { parentId: c.parentId })
    let attempts = 0
    const api = mockApi({
      ...inboxRoutes({ drafts: [d] }),
      [`POST ${c.createPath}`]: () => (++attempts === 1 ? apiError(400, "name taken") : { id: ids.created }),
    })
    const { user } = renderWithProviders(<InboxPage />)

    const accept = await findAccept(d)
    await waitFor(() => expect(accept).toBeEnabled())
    await user.click(accept)
    const dialog = await screen.findByRole("dialog", { name: c.dialogTitle })
    const field = within(dialog).getByDisplayValue(c.shown[0])
    await user.type(field, " typed")
    const typed = `${c.shown[0]} typed`
    await user.click(within(dialog).getByRole("button", { name: "Save" }))

    await waitFor(() => expect(errorToasts()).toEqual(["name taken"]))
    expect(screen.getByRole("dialog", { name: c.dialogTitle })).toBe(dialog)
    expect(within(dialog).getByDisplayValue(typed)).toBeInTheDocument()
    expect(api.callsTo("POST", `/drafts/${d.id}/accept`)).toHaveLength(0)

    const save = within(dialog).getByRole("button", { name: "Save" })
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
    const creates = api.callsTo("POST", c.createPath)
    expect(creates).toHaveLength(2)
    expect(Object.values(creates[1].body as Record<string, unknown>)).toContain(typed)
    expect(api.callsTo("POST", `/drafts/${d.id}/accept`)).toHaveLength(1)
    expect(await screen.findByText("Draft accepted and document linked")).toBeInTheDocument()
    expect(errorToasts()).toEqual(["name taken"])
  })

  it("does not accept the draft when creating the entry fails", async () => {
    const d = draft("vehicle", { name: "Family car" })
    const api = mockApi({ ...inboxRoutes({ drafts: [d] }), "POST /vehicles": apiError(400, "name taken") })
    const { user } = renderWithProviders(<InboxPage />)

    await user.click(await findAccept(d))
    const dialog = await screen.findByRole("dialog", { name: "Add Vehicle" })
    await user.click(within(dialog).getByRole("button", { name: "Save" }))

    await waitFor(() => expect(errorToasts()).toEqual(["name taken"]))
    expect(api.callsTo("POST", `/drafts/${d.id}/accept`)).toHaveLength(0)
    expect(screen.getByText(d.source.title!)).toBeInTheDocument()
  })

  it("says once that the entry exists when accepting the draft fails", async () => {
    const d = draft("vehicle", { name: "Family car" })
    mockApi({ ...inboxRoutes({ drafts: [d] }), [`POST /drafts/${d.id}/accept`]: apiError(409, "already decided") })
    const { user } = renderWithProviders(<InboxPage />)

    await user.click(await findAccept(d))
    const dialog = await screen.findByRole("dialog", { name: "Add Vehicle" })
    await user.click(within(dialog).getByRole("button", { name: "Save" }))

    await waitFor(() =>
      expect(errorToasts()).toEqual(["The entry was created, but the draft could not be accepted: already decided"]),
    )
    // the entity exists, so its create dialog does not stay open
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
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

describe("InboxPage bulk accept", () => {
  function named(type: Draft["targetType"], n: number, input: Record<string, unknown>, extra: Partial<Draft> = {}): Draft {
    return draft(type, input, { id: `${type}-${n}`, source: { kind: "mcp", ref: `ref-${type}-${n}`, title: `${type} ${n}` }, ...extra })
  }

  function purchases(count: number): Draft[] {
    return Array.from({ length: count }, (_, i) =>
      named("purchase", i + 1, { itemName: `Item ${i + 1}` }, { parentId: ids.purchaseCategory }),
    )
  }

  function findCheckbox(d: Draft) {
    return screen.findByRole("checkbox", { name: `Select ${d.source.title}` })
  }

  function batchCalls(api: ReturnType<typeof mockApi>) {
    return api.callsTo("POST", "/drafts/accept-batch")
  }

  async function confirm(user: ReturnType<typeof renderWithProviders>["user"], title: string) {
    await user.click(screen.getByRole("button", { name: "Accept selected" }))
    const dialog = await screen.findByRole("alertdialog", { name: title })
    await user.click(within(dialog).getByRole("button", { name: "Accept" }))
  }

  it("accepts the selected drafts with one request and without creating entries itself", async () => {
    const [first, second, third] = purchases(3)
    const api = mockApi(inboxRoutes({ drafts: [first, second, third] }))
    const { user } = renderWithProviders(<InboxPage />)

    const box = await findCheckbox(first)
    await waitFor(() => expect(box).toBeEnabled())
    expect(screen.getByRole("button", { name: "Accept selected" })).toBeDisabled()
    await user.click(box)
    await user.click(await findCheckbox(third))
    expect(screen.getByText("2 selected")).toBeInTheDocument()

    await confirm(user, "Accept 2 drafts?")

    expect(await screen.findByText("2 drafts accepted")).toBeInTheDocument()
    expect(batchCalls(api)).toHaveLength(1)
    expect(batchCalls(api)[0].body).toEqual({ ids: [first.id, third.id], baseUrl: window.location.origin })
    expect(api.calls.filter((call) => call.method === "POST")).toHaveLength(1)
    await waitFor(() => expect(screen.queryByText(first.source.title!)).not.toBeInTheDocument())
    expect(screen.queryByText(third.source.title!)).not.toBeInTheDocument()
    expect(screen.getByText(second.source.title!)).toBeInTheDocument()
  })

  it("sends nothing when the confirmation is cancelled", async () => {
    const [only] = purchases(1)
    const api = mockApi(inboxRoutes({ drafts: [only] }))
    const { user } = renderWithProviders(<InboxPage />)

    const box = await findCheckbox(only)
    await waitFor(() => expect(box).toBeEnabled())
    await user.click(box)
    await user.click(screen.getByRole("button", { name: "Accept selected" }))
    const dialog = await screen.findByRole("alertdialog", { name: "Accept 1 draft?" })
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
    expect(batchCalls(api)).toHaveLength(0)
    expect(box).toBeChecked()
  })

  it("only lets drafts be selected that Accept would take", async () => {
    const [ready] = purchases(1)
    const unclassified = named("none", 1, {})
    const noCategory = named("contract", 1, { name: "Fiber 500" })
    const disabledModule = named("vehicle", 1, { name: "Family car" })
    const api = mockApi(
      inboxRoutes({ drafts: [ready, unclassified, noCategory, disabledModule], enabledModules: ["contracts", "purchases", "ledger"] }),
    )
    const { user } = renderWithProviders(<InboxPage />)

    const box = await findCheckbox(ready)
    await waitFor(() => expect(box).toBeEnabled())
    for (const d of [unclassified, noCategory, disabledModule]) {
      expect(await findCheckbox(d)).toBeDisabled()
    }

    await user.click(screen.getByRole("checkbox", { name: "Select all (1)" }))
    expect(box).toBeChecked()
    await confirm(user, "Accept 1 draft?")

    await waitFor(() => expect(batchCalls(api)).toHaveLength(1))
    expect(batchCalls(api)[0].body).toMatchObject({ ids: [ready.id] })
  })

  it("selects all drafts of the filtered type", async () => {
    const bought = purchases(2)
    const vehicle = named("vehicle", 1, { name: "Family car" })
    const api = mockApi(inboxRoutes({ drafts: [bought[0], vehicle, bought[1]] }))
    const { user } = renderWithProviders(<InboxPage />)

    await waitFor(async () => expect(await findCheckbox(vehicle)).toBeEnabled())
    await user.click(await findCheckbox(vehicle))
    await user.click(screen.getByRole("combobox", { name: "Show" }))
    expect((await screen.findAllByRole("option")).map((o) => o.textContent)).toEqual(["All types (3)", "Purchase (2)", "Vehicle (1)"])
    await user.click(screen.getByRole("option", { name: "Purchase (2)" }))

    await waitFor(() => expect(screen.queryByText(vehicle.source.title!)).not.toBeInTheDocument())
    // the vehicle selected before the filter must not be accepted unseen
    expect(screen.getByRole("button", { name: "Accept selected" })).toBeDisabled()
    await user.click(screen.getByRole("checkbox", { name: "Select all (2)" }))
    await confirm(user, "Accept 2 drafts?")

    await waitFor(() => expect(batchCalls(api)).toHaveLength(1))
    expect(batchCalls(api)[0].body).toMatchObject({ ids: bought.map((d) => d.id) })
    // the filter's last draft is gone, so the other types show again
    expect(await screen.findByText(vehicle.source.title!)).toBeInTheDocument()
  })

  it("shows why a draft failed, keeps it selected and clears the reason when the draft is edited", async () => {
    const [good, bad, moved] = purchases(3)
    const api = mockApi(
      inboxRoutes({
        drafts: [good, bad, moved],
        batchFailures: {
          [bad.id]: { code: "invalid_input", error: "itemName is required" },
          [moved.id]: { code: "parent_not_found", error: "the draft's category or vehicle does not exist" },
        },
      }),
    )
    api.routes[`PUT /drafts/${bad.id}`] = () => bad
    const { user } = renderWithProviders(<InboxPage />)

    await waitFor(async () => expect(await findCheckbox(good)).toBeEnabled())
    await user.click(screen.getByRole("checkbox", { name: "Select all (3)" }))
    await confirm(user, "Accept 3 drafts?")

    expect(await screen.findByText("1 draft accepted")).toBeInTheDocument()
    expect(await screen.findByText("2 drafts could not be accepted, see the reason on each draft")).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText(good.source.title!)).not.toBeInTheDocument())
    expect(within(card(bad)).getByRole("alert")).toHaveTextContent("The entry could not be created: itemName is required")
    expect(within(card(moved)).getByRole("alert")).toHaveTextContent("The category or vehicle of this draft no longer exists.")
    expect(await findCheckbox(bad)).toBeChecked()
    expect(screen.getByText("2 selected")).toBeInTheDocument()

    await user.click(within(card(bad)).getByRole("combobox", { name: "Type" }))
    await user.click(await screen.findByRole("option", { name: "Vehicle" }))
    await waitFor(() => expect(within(card(bad)).queryByRole("alert")).not.toBeInTheDocument())
    expect(within(card(moved)).getByRole("alert")).toBeInTheDocument()
  })

  it("splits a large selection into several requests", async () => {
    const many = purchases(26)
    const api = mockApi(inboxRoutes({ drafts: many }))
    const { user } = renderWithProviders(<InboxPage />)

    // 26 cards take longer than the default second on a busy CI runner
    await waitFor(async () => expect(await findCheckbox(many[0])).toBeEnabled(), { timeout: 5000 })
    await user.click(screen.getByRole("checkbox", { name: "Select all (26)" }))
    await confirm(user, "Accept 26 drafts?")

    expect(await screen.findByText("26 drafts accepted", {}, { timeout: 5000 })).toBeInTheDocument()
    const sent = batchCalls(api).map((call) => (call.body as { ids: string[] }).ids)
    expect(sent.map((chunk) => chunk.length)).toEqual([25, 1])
    expect(sent.flat()).toEqual(many.map((d) => d.id))
    expect(await screen.findByText("No drafts waiting for review.", {}, { timeout: 5000 })).toBeInTheDocument()
  }, 20_000)

  it("reports a failed request and keeps the drafts selected", async () => {
    const [first, second] = purchases(2)
    const api = mockApi({ ...inboxRoutes({ drafts: [first, second] }), "POST /drafts/accept-batch": apiError(500, "internal error") })
    const { user } = renderWithProviders(<InboxPage />)

    await waitFor(async () => expect(await findCheckbox(first)).toBeEnabled())
    await user.click(screen.getByRole("checkbox", { name: "Select all (2)" }))
    await confirm(user, "Accept 2 drafts?")

    expect(
      await screen.findByText("The request failed, the remaining drafts were not processed: internal error"),
    ).toBeInTheDocument()
    expect(batchCalls(api)).toHaveLength(1)
    expect(screen.getByText("2 selected")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Accept selected" })).toBeEnabled()
  })
})
