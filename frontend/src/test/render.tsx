import type { ReactElement, ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import type { QueryClient } from "@tanstack/react-query"
import { render } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { Toaster } from "sonner"
import { createQueryClient } from "@/lib/query-client"

// The app's client (with the global mutation error toast), without retries.
export function createTestQueryClient(): QueryClient {
  return createQueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
}

export function queryWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

export function renderWithProviders(ui: ReactElement, queryClient = createTestQueryClient()) {
  const user = userEvent.setup()
  const result = render(
    <QueryClientProvider client={queryClient}>
      {ui}
      <Toaster />
    </QueryClientProvider>,
  )
  return { ...result, user, queryClient }
}
