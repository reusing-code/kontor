import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  getAllPurchases,
  getPurchaseById,
  getPurchasesByCategory,
  createPurchase,
  updatePurchase,
  deletePurchase,
} from "@/modules/purchases/lib/purchase-repository"
import type { PurchaseFormData } from "@/modules/purchases/types"

const purchasesKey = (categoryId: string) => ["purchases", categoryId] as const

export function useCategoryPurchases(categoryId: string) {
  return useQuery({
    queryKey: purchasesKey(categoryId),
    queryFn: () => getPurchasesByCategory(categoryId),
  })
}

export function usePurchases() {
  return useQuery({
    queryKey: ["purchases"],
    queryFn: getAllPurchases,
  })
}

export function usePurchase(id: string) {
  return useQuery({
    queryKey: ["purchases", id],
    queryFn: () => getPurchaseById(id),
  })
}

export function useCreatePurchase(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: PurchaseFormData) => createPurchase(categoryId, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: purchasesKey(categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "purchases"] })
      qc.invalidateQueries({ queryKey: ["purchases-summary"] })
    },
  })
}

export function useCreatePurchaseByCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ categoryId, data }: { categoryId: string; data: PurchaseFormData }) => createPurchase(categoryId, data),
    onSuccess: (purchase) => {
      qc.invalidateQueries({ queryKey: ["purchases"] })
      qc.invalidateQueries({ queryKey: purchasesKey(purchase.categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "purchases"] })
      qc.invalidateQueries({ queryKey: ["purchases-summary"] })
    },
  })
}

export function useUpdatePurchase(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: PurchaseFormData }) => updatePurchase(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["purchases"] })
      qc.invalidateQueries({ queryKey: purchasesKey(categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "purchases"] })
      qc.invalidateQueries({ queryKey: ["purchases-summary"] })
    },
  })
}

export function useUpdatePurchaseById() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: PurchaseFormData }) => updatePurchase(id, data),
    onSuccess: (purchase) => {
      qc.invalidateQueries({ queryKey: ["purchases"] })
      qc.invalidateQueries({ queryKey: ["purchases", purchase.id] })
      qc.invalidateQueries({ queryKey: ["categories", "purchases"] })
      qc.invalidateQueries({ queryKey: ["purchases-summary"] })
    },
  })
}

export function useDeletePurchase(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deletePurchase(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["purchases"] })
      qc.invalidateQueries({ queryKey: purchasesKey(categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "purchases"] })
      qc.invalidateQueries({ queryKey: ["purchases-summary"] })
    },
  })
}
