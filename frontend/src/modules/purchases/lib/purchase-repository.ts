import type { Purchase, PurchaseFormData, PurchaseSummary } from "@/modules/purchases/types"
import { del, get, post, put } from "@/lib/api"

export async function getAllPurchases(): Promise<Purchase[]> {
  return get<Purchase[]>("/purchases")
}

export async function getPurchasesByCategory(categoryId: string): Promise<Purchase[]> {
  return get<Purchase[]>(`/categories/${categoryId}/purchases`)
}

export async function getPurchaseById(id: string): Promise<Purchase> {
  return get<Purchase>(`/purchases/${id}`)
}

export async function createPurchase(categoryId: string, data: PurchaseFormData): Promise<Purchase> {
  return post<Purchase>(`/categories/${categoryId}/purchases`, data)
}

export async function updatePurchase(id: string, data: PurchaseFormData): Promise<Purchase> {
  return put<Purchase>(`/purchases/${id}`, data)
}

export async function deletePurchase(id: string): Promise<void> {
  return del(`/purchases/${id}`)
}

export async function getPurchaseSummary(): Promise<PurchaseSummary> {
  return get<PurchaseSummary>("/purchases/summary")
}
