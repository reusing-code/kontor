import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  getAllCategories,
  createCategory,
  updateCategory,
  deleteCategory,
} from "@/lib/category-repository"
import type { CategoryFormData } from "@/types/category"

const categoriesKey = (module: string) => ["categories", module] as const

export function useCategories(module: string) {
  return useQuery({
    queryKey: categoriesKey(module),
    queryFn: () => getAllCategories(module),
  })
}

export function useCreateCategory(module: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CategoryFormData) => createCategory(module, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: categoriesKey(module) }),
  })
}

export function useUpdateCategory(module: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: CategoryFormData }) => updateCategory(module, id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: categoriesKey(module) }),
  })
}

export function useDeleteCategory(module: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteCategory(module, id),
    onSuccess: () => {
      const summaryKey = module === "contracts" ? ["summary"] : module === "purchases" ? ["purchases-summary"] : null
      return Promise.all([
        qc.invalidateQueries({ queryKey: categoriesKey(module) }),
        summaryKey && qc.invalidateQueries({ queryKey: summaryKey }),
      ])
    },
  })
}
