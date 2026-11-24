import { MutationCache, QueryClient } from "@tanstack/react-query"
import type { QueryClientConfig } from "@tanstack/react-query"
import { toast } from "sonner"
import i18n from "@/i18n"

export interface KontorMutationMeta extends Record<string, unknown> {
  // false: the caller shows the error itself (inline or with its own toast).
  errorToast?: boolean
  // Turns the error into the toast text, e.g. for translated endpoint codes.
  errorMessage?: (error: Error) => string
}

declare module "@tanstack/react-query" {
  interface Register {
    mutationMeta: KontorMutationMeta
  }
}

export function defaultErrorMessage(error: Error): string {
  return error.message || i18n.t("common.error")
}

// Every failed mutation shows one error toast, unless its meta opts out.
export function createQueryClient(config: QueryClientConfig = {}): QueryClient {
  return new QueryClient({
    ...config,
    mutationCache: new MutationCache({
      onError: (error, _variables, _onMutateResult, mutation) => {
        const meta = mutation.meta
        if (meta?.errorToast === false) return
        toast.error((meta?.errorMessage ?? defaultErrorMessage)(error))
      },
    }),
  })
}

export const queryClient = createQueryClient()
