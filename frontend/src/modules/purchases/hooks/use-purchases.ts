import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { categoryKeys } from "@/lib/query-keys"
import { purchaseKeys } from "@/modules/purchases/keys"
import {
  getAllPurchases,
  getPurchaseById,
  getPurchasesByCategory,
  createPurchase,
  updatePurchase,
  deletePurchase,
} from "@/modules/purchases/lib/purchase-repository"
import type { PurchaseFormData } from "@/modules/purchases/types"

// An item shows up in the lists, the detail, the summary and (contracts) the
// renewals, so every change refreshes the whole module.
const invalidate = (qc: QueryClient) =>
  Promise.all([
    qc.invalidateQueries({ queryKey: purchaseKeys.all }),
    qc.invalidateQueries({ queryKey: categoryKeys("purchases").all }),
  ])

export function useCategoryPurchases(categoryId: string) {
  return useQuery({
    queryKey: purchaseKeys.byCategory(categoryId),
    queryFn: () => getPurchasesByCategory(categoryId),
  })
}

export function usePurchases() {
  return useQuery({
    queryKey: purchaseKeys.lists(),
    queryFn: getAllPurchases,
  })
}

export function usePurchase(id: string) {
  return useQuery({
    queryKey: purchaseKeys.detail(id),
    queryFn: () => getPurchaseById(id),
  })
}

export function useCreatePurchase(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: PurchaseFormData) => createPurchase(categoryId, data),
    onSuccess: () => invalidate(qc),
  })
}

export function useCreatePurchaseByCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ categoryId, data }: { categoryId: string; data: PurchaseFormData }) => createPurchase(categoryId, data),
    onSuccess: () => invalidate(qc),
  })
}

export function useUpdatePurchase() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: PurchaseFormData }) => updatePurchase(id, data),
    onSuccess: () => invalidate(qc),
  })
}

export function useDeletePurchase() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deletePurchase(id),
    onSuccess: () => invalidate(qc),
  })
}
