import type { ReactElement } from "react"
import { describe, expect, it } from "vitest"
import { useMutation } from "@tanstack/react-query"
import { screen, waitFor } from "@testing-library/react"
import { toast } from "sonner"
import type { KontorMutationMeta } from "@/lib/query-client"
import { renderWithProviders } from "@/test/render"
import { errorToasts } from "@/test/toasts"

function Failing({ error, meta }: { error: Error; meta?: KontorMutationMeta }) {
  const mutation = useMutation({ mutationFn: () => Promise.reject(error), meta })
  return (
    <>
      <button type="button" onClick={() => mutation.mutate()}>
        Run
      </button>
      {mutation.isError && <p>failed</p>}
    </>
  )
}

async function run(ui: ReactElement) {
  const { user } = renderWithProviders(ui)
  await user.click(screen.getByRole("button", { name: "Run" }))
  await screen.findByText("failed")
}

describe("global mutation error toast", () => {
  it("shows one error toast with the error's message", async () => {
    await run(<Failing error={new Error("name taken")} />)
    await waitFor(() => expect(errorToasts()).toEqual(["name taken"]))
  })

  it("shows nothing with errorToast false", async () => {
    await run(<Failing error={new Error("name taken")} meta={{ errorToast: false }} />)
    // A toast of the mutation would have been queued before this one.
    toast.error("marker")
    await waitFor(() => expect(errorToasts()).toEqual(["marker"]))
  })

  it("uses errorMessage for the text", async () => {
    await run(<Failing error={new Error("busy")} meta={{ errorMessage: (e) => `custom: ${e.message}` }} />)
    await waitFor(() => expect(errorToasts()).toEqual(["custom: busy"]))
  })

  it("falls back to a generic text for an empty message", async () => {
    await run(<Failing error={new Error("")} />)
    await waitFor(() => expect(errorToasts()).toEqual(["Something went wrong."]))
  })
})
