import { describe, expect, it } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { PaperlessSearchDialog } from "@/components/paperless/paperless-search-dialog"

const pageSize = 25

function searchPage(call: ApiCall) {
  const params = new URLSearchParams(call.path.split("?")[1])
  const page = Number(params.get("page"))
  const prefix = params.get("query") ?? "Doc"
  return {
    count: 120,
    page,
    pageSize,
    results: [{ id: page, title: `${prefix} on page ${page}`, created: "2026-11-01" }],
  }
}

function setup() {
  const api = mockApi({ "GET /paperless/search": searchPage })
  const view = renderWithProviders(
    <PaperlessSearchDialog open onOpenChange={() => {}} entityType="purchase" entityId="p1" entityUrl="http://kontor/purchases/p1" existingDocIds={[]} />,
  )
  return { api, ...view }
}

const searches = (api: ReturnType<typeof mockApi>) => api.calls.filter((call) => call.path.startsWith("/paperless/search")).map((call) => call.path)

describe("PaperlessSearchDialog paging", () => {
  it("jumps to a page and steps back", async () => {
    const { api, user } = setup()

    expect(await screen.findByText("Doc on page 1")).toBeInTheDocument()
    const nav = screen.getByRole("navigation", { name: "Search result pages" })
    expect(within(nav).getAllByRole("button", { name: /^Page \d+$/ }).map((b) => b.textContent)).toEqual(["1", "2", "5"])

    await user.click(within(nav).getByRole("button", { name: "Page 5" }))
    expect(await screen.findByText("Doc on page 5")).toBeInTheDocument()
    expect(within(nav).getByRole("button", { name: "Next page" })).toBeDisabled()

    await user.click(within(nav).getByRole("button", { name: "Previous page" }))
    expect(await screen.findByText("Doc on page 4")).toBeInTheDocument()
    expect(searches(api)).toEqual(["/paperless/search?page=1", "/paperless/search?page=5", "/paperless/search?page=4"])
  })

  it("starts a new search at page 1", async () => {
    const { api, user } = setup()

    await screen.findByText("Doc on page 1")
    await user.click(screen.getByRole("button", { name: "Page 5" }))
    await screen.findByText("Doc on page 5")

    await user.type(screen.getByPlaceholderText("Search documents…"), "tax")
    expect(await screen.findByText("tax on page 1")).toBeInTheDocument()
    await waitFor(() => expect(searches(api).at(-1)).toBe("/paperless/search?page=1&query=tax"))
  })
})
