import { describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { apiError, mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { LedgerEmailAccountDialog } from "@/modules/ledger/components/ledger-email-account-dialog"
import { visibleFolders } from "@/modules/ledger/lib/email-folders"
import type { LedgerEmailAccount } from "@/modules/ledger/types"

const foldersRoute = "POST /ledger/email-accounts/folders"
const folders = ["INBOX", "[Gmail]/Entwürfe", "archive", "Orders"]

const account = {
  id: "7d0a1f6e-6a59-4b3a-9c0b-0d5c8f0f3a11",
  name: "Mail",
  imapHost: "imap.example.com",
  imapPort: 993,
  username: "me@example.com",
  useTls: true,
  folder: "Orders",
  preClassify: false,
  scanSince: "2026-01-01",
} as LedgerEmailAccount

function renderDialog(routes: Record<string, unknown>, existing?: LedgerEmailAccount) {
  const api = mockApi({ "GET /capabilities": { llm: false, llmVision: false, bankImport: false }, ...routes })
  const onSubmit = vi.fn()
  const view = renderWithProviders(
    <LedgerEmailAccountDialog open onOpenChange={() => {}} account={existing} onSubmit={onSubmit} />,
  )
  return { ...view, api, onSubmit }
}

const folderInput = () => screen.getByPlaceholderText<HTMLInputElement>("INBOX")
const folderList = () => screen.getByRole("list", { name: "Folder / label" })
const listed = () => within(folderList()).getAllByRole("button").map((b) => b.textContent)

describe("visibleFolders", () => {
  it("shows everything for an empty field or a listed name, otherwise filters", () => {
    expect(visibleFolders(folders, "  ")).toEqual(folders)
    expect(visibleFolders(folders, "inbox")).toEqual(folders)
    expect(visibleFolders(folders, "gmail")).toEqual(["[Gmail]/Entwürfe"])
    expect(visibleFolders(folders, "R")).toEqual(["[Gmail]/Entwürfe", "archive", "Orders"])
    expect(visibleFolders(folders, "nope")).toEqual([])
  })
})

describe("LedgerEmailAccountDialog folder list", () => {
  it("loads the folders with the stored password of an edited account and fills the input on click", async () => {
    const { user, api } = renderDialog({ [foldersRoute]: { folders } }, account)
    expect(screen.queryByRole("list", { name: "Folder / label" })).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Load folders" }))
    await waitFor(() => expect(listed()).toEqual(folders))
    expect(api.callsTo("POST", "/ledger/email-accounts/folders")[0].body).toEqual({
      accountId: account.id,
      imapHost: "imap.example.com",
      imapPort: 993,
      username: "me@example.com",
      useTls: true,
    })
    expect(within(folderList()).getByRole("button", { name: "Orders" })).toHaveAttribute("aria-pressed", "true")

    await user.click(within(folderList()).getByRole("button", { name: "[Gmail]/Entwürfe" }))
    expect(folderInput()).toHaveValue("[Gmail]/Entwürfe")
    expect(listed()).toEqual(folders)
  })

  it("sends the entered password for a new account and filters by what is typed", async () => {
    const { user, api } = renderDialog({ [foldersRoute]: { folders } })
    const dialog = screen.getByRole("dialog")
    await user.type(within(dialog).getByLabelText("Username"), "me@example.com")
    await user.type(within(dialog).getByLabelText("Password"), "s3cret")
    await user.click(screen.getByRole("button", { name: "Load folders" }))
    await waitFor(() => expect(listed()).toEqual(folders))
    expect(api.callsTo("POST", "/ledger/email-accounts/folders")[0].body).toEqual({
      imapHost: "imap.gmail.com",
      imapPort: 993,
      username: "me@example.com",
      password: "s3cret",
      useTls: true,
    })

    await user.clear(folderInput())
    await user.type(folderInput(), "arch")
    expect(listed()).toEqual(["archive"])
    await user.type(folderInput(), "xyz")
    expect(screen.queryByRole("list", { name: "Folder / label" })).not.toBeInTheDocument()
    expect(screen.getByText(/No folder matches/)).toBeInTheDocument()
  })

  it.each([
    ["imap_login_failed", /refused the login/],
    ["imap_unavailable", /could not be loaded from the mail server/],
    ["password_missing", /Enter the password to load the folders/],
    [undefined, /could not be loaded\. Enter the folder name by hand/],
  ])("keeps the free-text input usable when listing fails (%s)", async (code, message) => {
    const { user, onSubmit } = renderDialog({ [foldersRoute]: apiError(502, "upstream text", code) }, account)
    await user.click(screen.getByRole("button", { name: "Load folders" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(message)
    expect(screen.queryByText(/upstream text/)).not.toBeInTheDocument()

    await user.clear(folderInput())
    await user.type(folderInput(), "Typed/By Hand")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onSubmit).toHaveBeenCalled())
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ folder: "Typed/By Hand" })
    expect(onSubmit.mock.calls[0][0]).not.toHaveProperty("password")
  })

  it("needs a password for a new account and keeps the stored one when editing", async () => {
    const { user, onSubmit } = renderDialog({})
    const dialog = screen.getByRole("dialog")
    await user.type(within(dialog).getByLabelText("Account label"), "Mail")
    await user.type(within(dialog).getByLabelText("Username"), "me@example.com")
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(await screen.findByText("Enter the password.")).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()

    await user.type(within(dialog).getByLabelText("Password"), "s3cret")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onSubmit).toHaveBeenCalled())
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ password: "s3cret", folder: "INBOX" })
  })

  it("says so when the server lists no folders", async () => {
    const { user } = renderDialog({ [foldersRoute]: { folders: [] } }, account)
    await user.click(screen.getByRole("button", { name: "Load folders" }))
    expect(await screen.findByText(/lists no folders/)).toBeInTheDocument()
  })
})

describe("LedgerEmailAccountDialog Kontor inbox folder", () => {
  const withLlm = { "GET /capabilities": { llm: true, llmVision: false, bankImport: false } }
  const draftsInput = () => screen.findByRole<HTMLInputElement>("textbox", { name: "Folder for the Kontor inbox (optional)" })

  it("is hidden without the LLM and keeps the stored value", async () => {
    const { user, onSubmit } = renderDialog({}, { ...account, draftsFolder: "Kontor" })
    expect(screen.queryByRole("textbox", { name: "Folder for the Kontor inbox (optional)" })).not.toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onSubmit).toHaveBeenCalled())
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ folder: "Orders", draftsFolder: "Kontor" })
  })

  it("refuses the order folder and saves another one picked from the list", async () => {
    const { user, onSubmit } = renderDialog({ ...withLlm, [foldersRoute]: { folders } }, account)
    await user.type(await draftsInput(), "Orders")
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(await screen.findByText("Choose a folder other than the one for order emails.")).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()

    await user.click(screen.getByRole("button", { name: "Load folders" }))
    const list = await screen.findByRole("list", { name: "Folder for the Kontor inbox (optional)" })
    await user.clear(await draftsInput())
    await user.click(within(list).getByRole("button", { name: "archive" }))
    expect(await draftsInput()).toHaveValue("archive")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onSubmit).toHaveBeenCalled())
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ folder: "Orders", draftsFolder: "archive" })
  })

  it("can be left empty", async () => {
    const { user, onSubmit } = renderDialog(withLlm, { ...account, draftsFolder: "Kontor" })
    await user.clear(await draftsInput())
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onSubmit).toHaveBeenCalled())
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ draftsFolder: "" })
  })
})
