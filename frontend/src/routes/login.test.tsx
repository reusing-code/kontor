import { describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { LoginPage } from "@/routes/login"
import { AuthProvider } from "@/hooks/use-auth"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => vi.fn(),
}))

function renderLogin() {
  return renderWithProviders(
    <AuthProvider>
      <LoginPage />
    </AuthProvider>,
  )
}

describe("LoginPage registration", () => {
  it("offers sign up while registration is open", async () => {
    const api = mockApi({ "GET /auth/config": { registrationOpen: true } })
    const { user } = renderLogin()

    const toggle = await screen.findByRole("button", { name: "Don't have an account? Sign up" })
    await user.click(toggle)
    expect(screen.getByText("Create an account")).toBeInTheDocument()
    expect(screen.getByLabelText("Confirm Password")).toBeInTheDocument()
    expect(api.callsTo("GET", "/auth/config")).toHaveLength(1)
  })

  it("hides sign up when registration is closed", async () => {
    const api = mockApi({ "GET /auth/config": { registrationOpen: false } })
    renderLogin()

    await waitFor(() => expect(api.callsTo("GET", "/auth/config")).toHaveLength(1))
    expect(screen.getByText("Sign in to your account")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Don't have an account? Sign up" })).not.toBeInTheDocument()
    expect(screen.queryByLabelText("Confirm Password")).not.toBeInTheDocument()
  })

  it("hides sign up when the settings cannot be loaded", async () => {
    mockApi({})
    renderLogin()

    expect(await screen.findByText("Sign in to your account")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Don't have an account? Sign up" })).not.toBeInTheDocument()
  })
})
