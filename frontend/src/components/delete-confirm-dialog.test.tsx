import { useState } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

function Harness({ onConfirm }: { onConfirm: () => void | Promise<unknown> }) {
  const [open, setOpen] = useState(true)
  return <DeleteConfirmDialog open={open} onOpenChange={setOpen} description="Delete it?" onConfirm={onConfirm} />
}

describe("DeleteConfirmDialog", () => {
  it("closes at once when onConfirm returns nothing", async () => {
    const onConfirm = vi.fn()
    const { user } = renderWithProviders(<Harness onConfirm={onConfirm} />)

    await user.click(screen.getByRole("button", { name: "Delete" }))

    expect(onConfirm).toHaveBeenCalledOnce()
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
  })

  it("stays open with disabled buttons until the promise resolves", async () => {
    let resolve = () => {}
    const onConfirm = vi.fn(() => new Promise<void>((r) => { resolve = r }))
    const { user } = renderWithProviders(<Harness onConfirm={onConfirm} />)

    await user.click(screen.getByRole("button", { name: "Delete" }))

    expect(screen.getByRole("alertdialog")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled()
    resolve()
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
    expect(onConfirm).toHaveBeenCalledOnce()
  })

  it("stays open when the promise rejects", async () => {
    const { user } = renderWithProviders(<Harness onConfirm={() => Promise.reject(new Error("nope"))} />)

    await user.click(screen.getByRole("button", { name: "Delete" }))

    await waitFor(() => expect(screen.getByRole("button", { name: "Delete" })).toBeEnabled())
    expect(screen.getByRole("alertdialog")).toBeInTheDocument()
  })
})
