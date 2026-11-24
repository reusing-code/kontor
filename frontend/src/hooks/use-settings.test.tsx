import { describe, expect, it } from "vitest"
import { act, renderHook, waitFor } from "@testing-library/react"
import { useChangePassword } from "@/hooks/use-settings"
import { getToken, setToken } from "@/lib/api"
import { mockApi } from "@/test/api-mock"
import { createTestQueryClient, queryWrapper } from "@/test/render"

describe("useChangePassword", () => {
  it("keeps the session with the token the change returns", async () => {
    setToken("old-token")
    const api = mockApi({ "PUT /settings/password": { token: "new-token" } })
    const { result } = renderHook(() => useChangePassword(), { wrapper: queryWrapper(createTestQueryClient()) })

    act(() => result.current.mutate({ currentPassword: "old-password", newPassword: "new-password" }))

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(api.callsTo("PUT", "/settings/password")[0].body).toEqual({ currentPassword: "old-password", newPassword: "new-password" })
    expect(getToken()).toBe("new-token")
  })
})
