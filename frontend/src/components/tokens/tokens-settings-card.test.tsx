import { describe, expect, it, vi } from "vitest"
import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import type { UserEvent } from "@testing-library/user-event"
import { TokensSettingsCard } from "@/components/tokens/tokens-settings-card"
import { apiError, mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { Token, TokenCreated } from "@/types/token"
import { tokenKeys } from "@/lib/query-keys"

const rawToken = "kpat_s3cr3t-raw-token-value"

const laptop: Token = {
  id: "t1",
  name: "Laptop",
  scopes: ["read", "drafts"],
  createdAt: "2026-01-10T12:00:00Z",
  expiresAt: "2999-06-01T12:00:00Z",
  lastUsedAt: "2026-02-03T12:00:00Z",
}
const old: Token = { id: "t2", name: "Old script", scopes: ["links"], createdAt: "2020-01-10T12:00:00Z", expiresAt: "2020-04-10T12:00:00Z" }
const forever: Token = { id: "t3", name: "Forever", scopes: ["read"], createdAt: "2026-03-01T12:00:00Z" }

// A server with a token list; the create response is the only place the raw token appears.
function tokenServer(initial: Token[] = []) {
  let tokens = initial
  return mockApi({
    "GET /tokens": () => tokens,
    "POST /tokens": ({ body }: ApiCall) => {
      const created: Token = { id: "new", createdAt: "2026-05-01T12:00:00Z", ...(body as Pick<Token, "name" | "scopes" | "expiresAt">) }
      tokens = [...tokens, created]
      return { ...created, token: rawToken } satisfies TokenCreated
    },
    "DELETE /tokens/t1": () => {
      tokens = tokens.filter((t) => t.id !== "t1")
    },
  })
}

async function openCreateDialog(user: UserEvent) {
  await user.click(screen.getByRole("button", { name: "Create token" }))
  return screen.findByRole("dialog", { name: "Create token" })
}

async function chooseExpiry(user: UserEvent, dialog: HTMLElement, option: string) {
  await user.click(within(dialog).getByRole("combobox", { name: "Expires" }))
  await user.click(await screen.findByRole("option", { name: option }))
}

async function createToken(user: UserEvent, name = "CI agent") {
  const dialog = await openCreateDialog(user)
  await user.type(within(dialog).getByLabelText("Name"), name)
  await user.click(within(dialog).getByRole("button", { name: "Create" }))
  return screen.findByRole("dialog", { name: "Your new token" })
}

// The dialog also has the corner X of DialogContent, labelled "Close" as well.
function footerClose(dialog: HTMLElement) {
  return within(dialog)
    .getAllByRole("button", { name: "Close" })
    .find((button) => button.dataset.slot === "button")!
}

function clickOutside() {
  const overlay = document.querySelector('[data-slot="dialog-overlay"]')!
  fireEvent.pointerDown(overlay)
  fireEvent.mouseDown(overlay)
  fireEvent.click(overlay)
}

describe("TokensSettingsCard list", () => {
  it("shows the loading state while the list is fetched", async () => {
    let release: (tokens: Token[]) => void = () => {}
    mockApi({ "GET /tokens": () => new Promise<Token[]>((resolve) => (release = resolve)) })
    renderWithProviders(<TokensSettingsCard />)

    expect(screen.getByText("Loading...")).toBeInTheDocument()
    expect(screen.queryByText("No tokens yet.")).not.toBeInTheDocument()

    await waitFor(() => expect(release).not.toHaveLength(0))
    release([])
    expect(await screen.findByText("No tokens yet.")).toBeInTheDocument()
    expect(screen.queryByText("Loading...")).not.toBeInTheDocument()
  })

  it("shows the error state with the server message", async () => {
    mockApi({ "GET /tokens": apiError(500, "database unavailable") })
    renderWithProviders(<TokensSettingsCard />)

    expect(await screen.findByText("Could not load the tokens: database unavailable")).toHaveAttribute("role", "alert")
    expect(screen.queryByText("No tokens yet.")).not.toBeInTheDocument()
    expect(screen.queryByText("Loading...")).not.toBeInTheDocument()
  })

  it("lists tokens with scopes, dates, never and the expired badge", async () => {
    tokenServer([laptop, old, forever])
    renderWithProviders(<TokensSettingsCard />)

    const laptopRow = (await screen.findByText("Laptop")).closest("tr")!
    expect(within(laptopRow).getByText("read")).toBeInTheDocument()
    expect(within(laptopRow).getByText("drafts")).toBeInTheDocument()
    expect(within(laptopRow).queryByText("links")).not.toBeInTheDocument()
    expect(within(laptopRow).getByText("Jan 10, 2026")).toBeInTheDocument()
    expect(within(laptopRow).getByText("Jun 1, 2999")).toBeInTheDocument()
    expect(within(laptopRow).getByText("Feb 3, 2026")).toBeInTheDocument()
    expect(within(laptopRow).queryByText("Expired")).not.toBeInTheDocument()

    const oldRow = screen.getByText("Old script").closest("tr")!
    expect(within(oldRow).getByText("Expired")).toBeInTheDocument()
    expect(within(oldRow).getByText("never")).toBeInTheDocument()

    const foreverRow = screen.getByText("Forever").closest("tr")!
    expect(within(foreverRow).getAllByText("never")).toHaveLength(2)
    expect(within(foreverRow).queryByText("Expired")).not.toBeInTheDocument()
  })
})

describe("TokensSettingsCard create", () => {
  it("defaults to the read scope and a 90 day expiry", async () => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date("2026-05-01T10:00:00.000Z"))
    const api = tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const dialog = await openCreateDialog(user)
    expect(within(dialog).getByRole("checkbox", { name: /^read/ })).toBeChecked()
    expect(within(dialog).getByRole("checkbox", { name: /^drafts/ })).not.toBeChecked()
    expect(within(dialog).getByRole("checkbox", { name: /^links/ })).not.toBeChecked()
    expect(within(dialog).getByRole("combobox", { name: "Expires" })).toHaveTextContent("In 90 days")
    expect(within(dialog).queryByRole("alert")).not.toBeInTheDocument()

    await user.type(within(dialog).getByLabelText("Name"), "  CI agent  ")
    await user.click(within(dialog).getByRole("button", { name: "Create" }))

    await screen.findByRole("dialog", { name: "Your new token" })
    expect(api.callsTo("POST", "/tokens").map((c) => c.body)).toEqual([
      { name: "CI agent", scopes: ["read"], expiresAt: "2026-07-30T10:00:00.000Z" },
    ])
  })

  it("sends the chosen scopes in canonical order and the chosen expiry", async () => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date("2026-05-01T10:00:00.000Z"))
    const api = tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const dialog = await openCreateDialog(user)
    await user.type(within(dialog).getByLabelText("Name"), "Linker")
    await user.click(within(dialog).getByRole("checkbox", { name: /^categorize/ }))
    await user.click(within(dialog).getByRole("checkbox", { name: /^links/ }))
    await user.click(within(dialog).getByRole("checkbox", { name: /^drafts/ }))
    await user.click(within(dialog).getByRole("checkbox", { name: /^read/ }))
    await chooseExpiry(user, dialog, "In 30 days")
    await user.click(within(dialog).getByRole("button", { name: "Create" }))

    await screen.findByRole("dialog", { name: "Your new token" })
    expect(api.callsTo("POST", "/tokens").map((c) => c.body)).toEqual([
      { name: "Linker", scopes: ["drafts", "links", "categorize"], expiresAt: "2026-05-31T10:00:00.000Z" },
    ])
  })

  it("warns about a token that never expires and sends no expiresAt", async () => {
    const api = tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const dialog = await openCreateDialog(user)
    await user.type(within(dialog).getByLabelText("Name"), "Forever")
    await chooseExpiry(user, dialog, "Never")
    expect(within(dialog).getByRole("alert")).toHaveTextContent("A token that never expires stays valid until you revoke it")

    await user.click(within(dialog).getByRole("button", { name: "Create" }))
    await screen.findByRole("dialog", { name: "Your new token" })
    const body = api.callsTo("POST", "/tokens")[0].body
    expect(body).toEqual({ name: "Forever", scopes: ["read"] })
    expect(body).not.toHaveProperty("expiresAt")
  })

  it("requires a name and at least one scope", async () => {
    const api = tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const dialog = await openCreateDialog(user)
    await user.click(within(dialog).getByRole("checkbox", { name: /^read/ }))
    await user.click(within(dialog).getByRole("button", { name: "Create" }))

    expect(within(dialog).getByText("A name is required")).toBeInTheDocument()
    expect(within(dialog).getByText("Select at least one scope")).toBeInTheDocument()
    expect(api.callsTo("POST", "/tokens")).toHaveLength(0)
  })

  it("rejects a custom date that is not in the future", async () => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 4, 1, 12))
    const api = tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const dialog = await openCreateDialog(user)
    await user.type(within(dialog).getByLabelText("Name"), "Dated")
    await chooseExpiry(user, dialog, "Custom date")
    const date = within(dialog).getByLabelText("Custom date")
    expect(date).toHaveAttribute("min", "2026-05-02")

    await user.click(within(dialog).getByRole("button", { name: "Create" }))
    expect(within(dialog).getByText("Choose a date in the future")).toBeInTheDocument()

    // Below min the browser's own validation blocks the submit.
    fireEvent.change(date, { target: { value: "2026-05-01" } })
    expect(date).toBeInvalid()
    await user.click(within(dialog).getByRole("button", { name: "Create" }))
    expect(api.callsTo("POST", "/tokens")).toHaveLength(0)
    expect(screen.queryByRole("dialog", { name: "Your new token" })).not.toBeInTheDocument()

    fireEvent.change(date, { target: { value: "2026-05-02" } })
    expect(date).toBeValid()
    await user.click(within(dialog).getByRole("button", { name: "Create" }))
    await screen.findByRole("dialog", { name: "Your new token" })
    expect(api.callsTo("POST", "/tokens")).toHaveLength(1)
  })

  it("starts with a fresh form on every opening", async () => {
    tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    let dialog = await openCreateDialog(user)
    await user.type(within(dialog).getByLabelText("Name"), "Draft name")
    await user.click(within(dialog).getByRole("checkbox", { name: /^links/ }))
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())

    dialog = await openCreateDialog(user)
    expect(within(dialog).getByLabelText("Name")).toHaveValue("")
    expect(within(dialog).getByRole("checkbox", { name: /^links/ })).not.toBeChecked()
  })

  it("keeps the dialog open and reports a failed create", async () => {
    mockApi({ "GET /tokens": [], "POST /tokens": apiError(400, "token limit reached") })
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const dialog = await openCreateDialog(user)
    await user.type(within(dialog).getByLabelText("Name"), "One too many")
    await user.click(within(dialog).getByRole("button", { name: "Create" }))

    expect(await screen.findByText("token limit reached")).toBeInTheDocument()
    expect(screen.getByRole("dialog", { name: "Create token" })).toBeInTheDocument()
    expect(screen.queryByRole("dialog", { name: "Your new token" })).not.toBeInTheDocument()
  })
})

describe("TokensSettingsCard reveal", () => {
  it("shows the raw token once and lists the new token without it", async () => {
    tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const reveal = await createToken(user)
    expect(within(reveal).getByLabelText("Token")).toHaveValue(rawToken)
    expect(within(reveal).getByRole("alert")).toHaveTextContent("This token will not be shown again")
    expect(screen.queryByRole("dialog", { name: "Create token" })).not.toBeInTheDocument()

    await user.click(footerClose(reveal))
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
    expect(await screen.findByText("CI agent")).toBeInTheDocument()
    expect(document.body.innerHTML).not.toContain(rawToken)
  })

  it("never writes the raw token to the query cache", async () => {
    const api = tokenServer()
    const { user, queryClient } = renderWithProviders(<TokensSettingsCard />)

    const reveal = await createToken(user)
    await waitFor(() => expect(api.callsTo("GET", "/tokens")).toHaveLength(2))
    await waitFor(() => expect(queryClient.getQueryData<Token[]>(tokenKeys.all)).toHaveLength(1))

    const cached = () => JSON.stringify(queryClient.getQueryCache().getAll().map((q) => [q.queryKey, q.state.data]))
    expect(cached()).toContain("CI agent")
    expect(cached()).not.toContain(rawToken)
    expect(queryClient.getQueryData<Token[]>(tokenKeys.all)![0]).not.toHaveProperty("token")

    await user.click(footerClose(reveal))
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
    expect(cached()).not.toContain(rawToken)
  })

  it("drops the raw token from the mutation cache once the dialog is closed", async () => {
    tokenServer()
    const { user, queryClient } = renderWithProviders(<TokensSettingsCard />)

    const reveal = await createToken(user)
    await user.click(footerClose(reveal))
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())

    await waitFor(() => expect(queryClient.getMutationCache().getAll()).toHaveLength(0))
  })

  it("stays open on Escape and on a click outside", async () => {
    tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    // Control: the create dialog does close both ways, so the events reach Radix.
    await openCreateDialog(user)
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
    await openCreateDialog(user)
    clickOutside()
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())

    const reveal = await createToken(user)
    await user.keyboard("{Escape}")
    clickOutside()
    await new Promise((resolve) => setTimeout(resolve, 50))

    expect(reveal).toBeInTheDocument()
    expect(within(reveal).getByLabelText("Token")).toHaveValue(rawToken)
  })

  it("copies the token to the clipboard", async () => {
    tokenServer()
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const reveal = await createToken(user)
    await user.click(within(reveal).getByRole("button", { name: "Copy" }))

    expect(await screen.findByText("Copied to clipboard")).toBeInTheDocument()
    expect(await navigator.clipboard.readText()).toBe(rawToken)
  })
})

describe("TokensSettingsCard revoke", () => {
  it("revokes a token after confirmation", async () => {
    const api = tokenServer([laptop, forever])
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const row = (await screen.findByText("Laptop")).closest("tr")!
    await user.click(within(row).getByRole("button", { name: "Revoke" }))
    const confirm = await screen.findByRole("alertdialog")
    expect(confirm).toHaveTextContent('Revoke the token "Laptop"?')
    expect(api.callsTo("DELETE", "/tokens/t1")).toHaveLength(0)

    await user.click(within(confirm).getByRole("button", { name: "Revoke" }))

    await waitFor(() => expect(screen.queryByText("Laptop")).not.toBeInTheDocument())
    expect(api.callsTo("DELETE", "/tokens/t1")).toHaveLength(1)
    expect(screen.getByText("Forever")).toBeInTheDocument()
    expect(await screen.findByText("Token revoked")).toBeInTheDocument()
  })

  it("keeps the token when the confirmation is cancelled", async () => {
    const api = tokenServer([laptop])
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const row = (await screen.findByText("Laptop")).closest("tr")!
    await user.click(within(row).getByRole("button", { name: "Revoke" }))
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
    expect(api.calls.filter((c) => c.method === "DELETE")).toHaveLength(0)
    expect(screen.getByText("Laptop")).toBeInTheDocument()
  })

  it("reports a failed revoke and keeps the token", async () => {
    mockApi({ "GET /tokens": [laptop], "DELETE /tokens/t1": apiError(500, "revoke exploded") })
    const { user } = renderWithProviders(<TokensSettingsCard />)

    const row = (await screen.findByText("Laptop")).closest("tr")!
    await user.click(within(row).getByRole("button", { name: "Revoke" }))
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Revoke" }))

    expect(await screen.findByText("revoke exploded")).toBeInTheDocument()
    expect(screen.getByText("Laptop")).toBeInTheDocument()
  })
})
