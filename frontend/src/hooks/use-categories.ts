import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  getAllCategories,
  getCategoryById,
  createCategory,
  updateCategory,
  deleteCategory,
} from "@/lib/category-repository"
import { categoryKeys, type CategoryItemKeys } from "@/lib/query-keys"
import { contractKeys } from "@/modules/contracts/keys"
import { purchaseKeys } from "@/modules/purchases/keys"
import type { CategoryFormData, CategoryKind } from "@/types/category"

const itemKeys: Record<CategoryKind, CategoryItemKeys> = { contracts: contractKeys, purchases: purchaseKeys }

export function categoriesQueryOptions(kind: CategoryKind) {
  return { queryKey: categoryKeys(kind).list(), queryFn: () => getAllCategories(kind) }
}

export function useCategories(kind: CategoryKind) {
  return useQuery(categoriesQueryOptions(kind))
}

export function useCategory(kind: CategoryKind, id: string) {
  return useQuery({
    queryKey: categoryKeys(kind).detail(id),
    queryFn: () => getCategoryById(kind, id),
  })
}

export function useCreateCategory(kind: CategoryKind) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CategoryFormData) => createCategory(kind, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: categoryKeys(kind).all }),
  })
}

// The category page and the dashboard summary show the category's name.
export function useUpdateCategory(kind: CategoryKind) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: CategoryFormData }) => updateCategory(kind, id, data),
    onSuccess: (_, { id }) =>
      Promise.all([
        qc.invalidateQueries({ queryKey: categoryKeys(kind).all }),
        qc.invalidateQueries({ queryKey: itemKeys[kind].byCategory(id) }),
        qc.invalidateQueries({ queryKey: itemKeys[kind].summary() }),
      ]),
  })
}

// Deleting a category takes its items with it, so all of the module's item
// queries are refreshed.
export function useDeleteCategory(kind: CategoryKind) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteCategory(kind, id),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: categoryKeys(kind).all }),
        qc.invalidateQueries({ queryKey: itemKeys[kind].all }),
      ]),
  })
}
