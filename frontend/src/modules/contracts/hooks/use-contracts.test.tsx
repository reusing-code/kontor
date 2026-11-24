import { describe, expect, it } from "vitest"
import { act, renderHook, waitFor } from "@testing-library/react"
import { useCategory, useUpdateCategory } from "@/hooks/use-categories"
import { useCategoryContracts, useContracts, useCreateContract, useUpcomingRenewals } from "@/modules/contracts/hooks/use-contracts"
import type { ContractFormData } from "@/modules/contracts/types"
import { mockApi } from "@/test/api-mock"
import { createTestQueryClient, queryWrapper } from "@/test/render"

const categoryId = "22222222-2222-4222-8222-222222222222"
const stamp = "2026-05-01T08:00:00Z"
const category = { id: categoryId, name: "Insurance", createdAt: stamp, updatedAt: stamp }

const form: ContractFormData = {
  name: "Phone",
  price: 10,
  billingInterval: "monthly",
  startDate: "2026-01-01",
  minimumDurationMonths: 12,
  extensionDurationMonths: 12,
  noticePeriodMonths: 1,
}

function render<T>(hook: () => T) {
  const queryClient = createTestQueryClient()
  return renderHook(hook, { wrapper: queryWrapper(queryClient) })
}

describe("contract and category invalidation", () => {
  it("refetches the contracts list and the upcoming renewals after creating a contract", async () => {
    const api = mockApi({
      "GET /contracts": [],
      "GET /contracts/upcoming-renewals": [],
      [`POST /categories/${categoryId}/contracts`]: { id: "c1", categoryId, ...form, createdAt: stamp, updatedAt: stamp },
    })
    const { result } = render(() => ({
      list: useContracts(),
      renewals: useUpcomingRenewals(30),
      create: useCreateContract(categoryId),
    }))
    await waitFor(() => expect(result.current.list.isSuccess && result.current.renewals.isSuccess).toBe(true))

    await act(() => result.current.create.mutateAsync(form))

    await waitFor(() => expect(api.callsTo("GET", "/contracts")).toHaveLength(2))
    expect(api.callsTo("GET", "/contracts/upcoming-renewals?days=30")).toHaveLength(2)
  })

  it("refetches the category's contracts and the category after renaming it", async () => {
    const api = mockApi({
      [`GET /categories/${categoryId}/contracts`]: [],
      [`GET /modules/contracts/categories/${categoryId}`]: category,
      [`PUT /modules/contracts/categories/${categoryId}`]: { ...category, name: "Insurances" },
    })
    const { result } = render(() => ({
      contracts: useCategoryContracts(categoryId),
      category: useCategory("contracts", categoryId),
      update: useUpdateCategory("contracts"),
    }))
    await waitFor(() => expect(result.current.contracts.isSuccess && result.current.category.isSuccess).toBe(true))

    await act(() => result.current.update.mutateAsync({ id: categoryId, data: { name: "Insurances" } }))

    await waitFor(() => expect(api.callsTo("GET", `/categories/${categoryId}/contracts`)).toHaveLength(2))
    expect(api.callsTo("GET", `/modules/contracts/categories/${categoryId}`)).toHaveLength(2)
  })
})
