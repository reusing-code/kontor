import { describe, expect, it } from "vitest"
import { screen, within } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { apiError, mockApi } from "@/test/api-mock"
import { PaperlessSettingsCard } from "@/components/paperless/paperless-settings-card"

const baseRoutes = {
  "GET /paperless/config": { configured: true, baseUrl: "https://paperless.example" },
  "GET /capabilities": { llm: false, llmVision: false, bankImport: false },
}

describe("PaperlessSettingsCard connection test", () => {
  it("shows a successful test inline next to the toast", async () => {
    mockApi({ ...baseRoutes, "POST /paperless/config/test": { ok: true } })
    const { user } = renderWithProviders(<PaperlessSettingsCard />)

    await user.click(await screen.findByRole("button", { name: "Test connection" }))

    const form = screen.getByRole("button", { name: "Save" }).closest("form")!
    expect(await within(form).findByRole("status")).toHaveTextContent("Connection to paperless succeeded")
    expect(within(form).queryByRole("alert")).toBeNull()
  })

  it("shows a failed test inline with the server's message", async () => {
    mockApi({ ...baseRoutes, "POST /paperless/config/test": apiError(502, "paperless answered 401") })
    const { user } = renderWithProviders(<PaperlessSettingsCard />)

    await user.click(await screen.findByRole("button", { name: "Test connection" }))

    const form = screen.getByRole("button", { name: "Save" }).closest("form")!
    expect(await within(form).findByRole("alert")).toHaveTextContent("Connection failed: paperless answered 401")
    expect(within(form).queryByRole("status")).toBeNull()
  })

  it("clears the result when the connection is edited", async () => {
    mockApi({ ...baseRoutes, "POST /paperless/config/test": { ok: true } })
    const { user } = renderWithProviders(<PaperlessSettingsCard />)

    await user.click(await screen.findByRole("button", { name: "Test connection" }))
    const form = (await screen.findByRole("button", { name: "Save" })).closest("form")!
    expect(await within(form).findByText("Connection to paperless succeeded")).toBeInTheDocument()

    await user.type(screen.getByLabelText("Base URL"), "/")

    expect(within(form).queryByText("Connection to paperless succeeded")).toBeNull()
  })
})
