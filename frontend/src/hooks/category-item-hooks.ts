import { useMutation, useQuery, useQueryClient, type QueryClient, type UseMutationResult, type UseQueryResult } from "@tanstack/react-query"
import { categoryKeys, type CategoryItemKeys } from "@/lib/query-keys"
import type { CategoryKind } from "@/types/category"

export interface CategoryItemRepository<T, F> {
  getAll: () => Promise<T[]>
  getByCategory: (categoryId: string) => Promise<T[]>
  getById: (id: string) => Promise<T>
  create: (categoryId: string, data: F) => Promise<T>
  update: (id: string, data: F) => Promise<T>
  remove: (id: string) => Promise<void>
}

export interface CategoryItemHooks<T, F> {
  // Refreshes everything an item change can affect: the module's lists,
  // details, summary (and module extras below its root) and its categories.
  invalidate: (qc: QueryClient) => Promise<unknown>
  useItems: () => UseQueryResult<T[]>
  useItemsByCategory: (categoryId: string) => UseQueryResult<T[]>
  useItem: (id: string) => UseQueryResult<T>
  useCreateItem: (categoryId: string) => UseMutationResult<T, Error, F>
  useCreateItemByCategory: () => UseMutationResult<T, Error, { categoryId: string; data: F }>
  useUpdateItem: () => UseMutationResult<T, Error, { id: string; data: F }>
  useDeleteItem: () => UseMutationResult<void, Error, string>
}

// createCategoryItemHooks builds the CRUD hooks of a module whose items are
// filed under the shared categories (contracts, purchases).
export function createCategoryItemHooks<T, F>({
  kind,
  keys,
  repository,
}: {
  kind: CategoryKind
  keys: CategoryItemKeys
  repository: CategoryItemRepository<T, F>
}): CategoryItemHooks<T, F> {
  const invalidate = (qc: QueryClient) =>
    Promise.all([
      qc.invalidateQueries({ queryKey: keys.all }),
      qc.invalidateQueries({ queryKey: categoryKeys(kind).all }),
    ])

  function useInvalidatingMutation<V, R>(mutationFn: (variables: V) => Promise<R>) {
    const qc = useQueryClient()
    return useMutation({ mutationFn, onSuccess: () => invalidate(qc) })
  }

  return {
    invalidate,
    useItems: () => useQuery({ queryKey: keys.lists(), queryFn: repository.getAll }),
    useItemsByCategory: (categoryId) =>
      useQuery({ queryKey: keys.byCategory(categoryId), queryFn: () => repository.getByCategory(categoryId) }),
    useItem: (id) => useQuery({ queryKey: keys.detail(id), queryFn: () => repository.getById(id) }),
    useCreateItem: (categoryId) => useInvalidatingMutation((data: F) => repository.create(categoryId, data)),
    useCreateItemByCategory: () =>
      useInvalidatingMutation(({ categoryId, data }: { categoryId: string; data: F }) => repository.create(categoryId, data)),
    useUpdateItem: () => useInvalidatingMutation(({ id, data }: { id: string; data: F }) => repository.update(id, data)),
    useDeleteItem: () => useInvalidatingMutation((id: string) => repository.remove(id)),
  }
}
