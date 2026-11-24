import { describe, expect, it } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { SettingsPage } from "@/routes/settings"
import { apiError, mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { errorToasts } from "@/test/toasts"

const backup = { format: "kontor-export", formatVersion: 2, modules: { contracts: { schemaVersion: 1, categories: [], contracts: [] } } }

function exportFile(content: unknown = backup, name = "kontor-export.json"): File {
  return new File([typeof content === "string" ? content : JSON.stringify(content)], name, { type: "application/json" })
}

// The settings the page shows change once an import succeeded, as after a
// real restore.
function setup({ warnings = [], restoreError }: { warnings?: string[]; restoreError?: unknown } = {}) {
  let renewalDays = 30
  const changeSettings = () => {
    renewalDays = 45
    return { restored: { contracts: 0 }, warnings }
  }
  const api = mockApi({
    "GET /settings": () => ({ renewalDays, reminderFrequency: "weekly", enabledModules: ["contracts", "auto"] }),
    "GET /capabilities": { llm: false, llmVision: false, bankImport: false },
    "GET /paperless/config": { configured: false },
    "GET /tokens": [],
    "POST /import": restoreError ?? changeSettings,
    "POST /modules/contracts/import": changeSettings,
    "POST /modules/auto/import": changeSettings,
  })
  return { api, ...renderWithProviders(<SettingsPage />) }
}

async function renewalDaysInput() {
  const input = await screen.findByLabelText("Renewal timeframe (days)")
  await waitFor(() => expect(input).toHaveValue(30))
  return input
}

describe("SettingsPage restore", () => {
  it("asks before a restore and sends nothing on cancel", async () => {
    const { api, user } = setup()
    await renewalDaysInput()

    await user.upload(screen.getByLabelText("Restore from export"), exportFile())
    const dialog = await screen.findByRole("alertdialog")
    expect(within(dialog).getByText(/Restore from "kontor-export.json"\? Your settings and the categories/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
    expect(api.calls.filter((c) => c.method === "POST")).toHaveLength(0)
  })

  it("uploads the file after confirming and shows the refetched data", async () => {
    const { api, user } = setup({ warnings: ["paperless links were not imported"] })
    const input = await renewalDaysInput()
    expect(api.callsTo("GET", "/settings")).toHaveLength(1)

    await user.upload(screen.getByLabelText("Restore from export"), exportFile())
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Restore" }))

    expect(await screen.findByText("Data restored successfully.")).toBeInTheDocument()
    expect(screen.getByText("paperless links were not imported")).toBeInTheDocument()
    expect(api.callsTo("POST", "/import").map((c) => c.body)).toEqual([backup])
    await waitFor(() => expect(input).toHaveValue(45))
    expect(api.callsTo("GET", "/settings")).toHaveLength(2)
    expect(errorToasts()).toEqual([])
  })

  it("shows one error toast when the restore fails", async () => {
    const { api, user } = setup({ restoreError: apiError(409, "import requires the contracts module to have no data") })
    const input = await renewalDaysInput()

    await user.upload(screen.getByLabelText("Restore from export"), exportFile())
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Restore" }))

    await waitFor(() => expect(errorToasts()).toEqual(["import requires the contracts module to have no data"]))
    expect(api.callsTo("POST", "/import")).toHaveLength(1)
    expect(screen.queryByText("Data restored successfully.")).not.toBeInTheDocument()
    expect(input).toHaveValue(30)
    expect(screen.getByRole("button", { name: "Restore from export" })).toBeEnabled()
  })

  it("sends nothing and shows one error toast for a file that is no JSON", async () => {
    const { api, user } = setup()
    await renewalDaysInput()

    await user.upload(screen.getByLabelText("Restore from export"), exportFile("not json"))
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Restore" }))

    await waitFor(() => expect(errorToasts()).toHaveLength(1))
    expect(api.calls.filter((c) => c.method === "POST")).toHaveLength(0)
  })
})

describe("SettingsPage module import", () => {
  it("asks before importing a module with categories and refetches afterwards", async () => {
    const { api, user } = setup()
    const input = await renewalDaysInput()

    await user.upload(screen.getByLabelText("Export file for Contracts"), exportFile(backup, "contracts.json"))
    const dialog = await screen.findByRole("alertdialog")
    expect(within(dialog).getByText(/Import Contracts data from "contracts.json"\? The categories of Contracts/)).toBeInTheDocument()
    expect(api.calls.filter((c) => c.method === "POST")).toHaveLength(0)
    await user.click(within(dialog).getByRole("button", { name: "Import" }))

    expect(await screen.findByText("Contracts data imported.")).toBeInTheDocument()
    expect(api.callsTo("POST", "/modules/contracts/import").map((c) => c.body)).toEqual([backup])
    await waitFor(() => expect(input).toHaveValue(45))
    expect(api.callsTo("GET", "/settings")).toHaveLength(2)
  })

  it("imports the auto module without asking, since nothing is replaced", async () => {
    const { api, user } = setup()
    await renewalDaysInput()

    await user.upload(screen.getByLabelText("Export file for Auto"), exportFile())

    expect(await screen.findByText("Auto data imported.")).toBeInTheDocument()
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument()
    expect(api.callsTo("POST", "/modules/auto/import")).toHaveLength(1)
    await waitFor(() => expect(api.callsTo("GET", "/settings")).toHaveLength(2))
  })
})
