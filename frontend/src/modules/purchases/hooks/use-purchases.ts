import { createCategoryItemHooks } from "@/hooks/category-item-hooks"
import { purchaseKeys } from "@/modules/purchases/keys"
import {
  getAllPurchases,
  getPurchaseById,
  getPurchasesByCategory,
  createPurchase,
  updatePurchase,
  deletePurchase,
} from "@/modules/purchases/lib/purchase-repository"
import type { Purchase, PurchaseFormData } from "@/modules/purchases/types"

export const purchaseHooks = createCategoryItemHooks<Purchase, PurchaseFormData>({
  kind: "purchases",
  keys: purchaseKeys,
  repository: {
    getAll: getAllPurchases,
    getByCategory: getPurchasesByCategory,
    getById: getPurchaseById,
    create: createPurchase,
    update: updatePurchase,
    remove: deletePurchase,
  },
})

export const usePurchases = purchaseHooks.useItems
export const useCategoryPurchases = purchaseHooks.useItemsByCategory
export const usePurchase = purchaseHooks.useItem
export const useCreatePurchase = purchaseHooks.useCreateItem
export const useCreatePurchaseByCategory = purchaseHooks.useCreateItemByCategory
export const useUpdatePurchase = purchaseHooks.useUpdateItem
export const useDeletePurchase = purchaseHooks.useDeleteItem
