import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { CategoryItemsPage } from "@/components/category-items/category-items-page"
import { CategorySidebarSection } from "@/components/category-items/category-sidebar-section"
import { contractItems } from "@/modules/contracts/config/category-items"
import { purchaseItems } from "@/modules/purchases/config/category-items"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  Link: ({ children, to, params }: { children: ReactNode; to: string; params?: Record<string, string> }) => (
    <a href={Object.entries(params ?? {}).reduce((path, [key, value]) => path.replace(`$${key}`, value), to)}>{children}</a>
  ),
  useMatchRoute: () => () => false,
}))

const categoryId = "22222222-2222-4222-8222-222222222222"
const itemId = "55555555-5555-4555-8555-555555555555"
const stamp = "2026-05-01T08:00:00Z"
const category = { id: categoryId, name: "Household", createdAt: stamp, updatedAt: stamp }
const base = { id: itemId, categoryId, createdAt: stamp, updatedAt: stamp }

const cases = [
  {
    kind: "contracts",
    render: () => <CategoryItemsPage config={contractItems} categoryId={categoryId} />,
    item: { ...base, name: "Phone", billingInterval: "monthly", startDate: "2026-01-01", minimumDurationMonths: 12, extensionDurationMonths: 12, noticePeriodMonths: 1 },
    name: "Phone",
    create: "Add Contract",
    nameLabel: /Contract Name/,
    created: { name: "Internet", billingInterval: "monthly" },
    deleteConfirm: 'This will permanently delete the contract "Phone".',
  },
  {
    kind: "purchases",
    render: () => <CategoryItemsPage config={purchaseItems} categoryId={categoryId} />,
    item: { ...base, itemName: "Kettle", purchaseDate: "2026-01-01" },
    name: "Kettle",
    create: "Add Purchase",
    nameLabel: /Item Name/,
    created: { itemName: "Internet" },
    deleteConfirm: 'This will permanently delete the purchase "Kettle".',
  },
]

describe("CategoryItemsPage", () => {
  it.each(cases)("lists, creates and deletes $kind", async (c) => {
    const api = mockApi({
      [`GET /modules/${c.kind}/categories/${categoryId}`]: category,
      [`GET /categories/${categoryId}/${c.kind}`]: [c.item],
      [`POST /categories/${categoryId}/${c.kind}`]: (call: { body: object }) => ({ ...base, id: "new", ...call.body }),
      [`DELETE /${c.kind}/${itemId}`]: undefined,
    })
    const { user } = renderWithProviders(c.render())

    expect(await screen.findByRole("heading", { name: "Household" })).toBeInTheDocument()
    const link = await screen.findByRole("link", { name: c.name })
    expect(link).toHaveAttribute("href", `/${c.kind}/${itemId}`)

    await user.click(screen.getByRole("button", { name: c.create }))
    const dialog = await screen.findByRole("dialog", { name: c.create })
    await user.type(within(dialog).getByLabelText(c.nameLabel), "Internet")
    await user.click(within(dialog).getByRole("button", { name: "Save" }))
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
    const creates = api.callsTo("POST", `/categories/${categoryId}/${c.kind}`)
    expect(creates).toHaveLength(1)
    expect(creates[0].body).toMatchObject(c.created)
    await waitFor(() => expect(api.callsTo("GET", `/categories/${categoryId}/${c.kind}`)).toHaveLength(2))

    const row = link.closest("tr")!
    const buttons = within(row).getAllByRole("button")
    await user.click(buttons[buttons.length - 1])
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    const confirm = await screen.findByRole("alertdialog")
    expect(confirm).toHaveTextContent(c.deleteConfirm)
    expect(api.callsTo("DELETE", `/${c.kind}/${itemId}`)).toHaveLength(0)
    await user.click(within(confirm).getByRole("button", { name: "Delete" }))
    await waitFor(() => expect(api.callsTo("DELETE", `/${c.kind}/${itemId}`)).toHaveLength(1))
  })
})

describe("CategorySidebarSection", () => {
  it.each([
    ["contracts", "Contracts"],
    ["purchases", "Purchases"],
  ] as const)("links %s and each of its categories", async (kind, title) => {
    mockApi({ [`GET /modules/${kind}/categories`]: [category] })
    renderWithProviders(<CategorySidebarSection kind={kind} />)

    expect(screen.getByRole("link", { name: title })).toHaveAttribute("href", `/${kind}`)
    expect(await screen.findByRole("link", { name: "Household" })).toHaveAttribute("href", `/${kind}/categories/${categoryId}`)
  })
})
