import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { categoryKeys } from "@/lib/query-keys"
import { contractKeys } from "@/modules/contracts/keys"
import {
  getAllContracts,
  getContractById,
  getContractsByCategory,
  getUpcomingRenewals,
  createContract,
  updateContract,
  deleteContract,
  importContracts,
} from "@/modules/contracts/lib/contract-repository"
import type { ContractFormData } from "@/modules/contracts/types"

// An item shows up in the lists, the detail, the summary and (contracts) the
// renewals, so every change refreshes the whole module.
const invalidate = (qc: QueryClient) =>
  Promise.all([
    qc.invalidateQueries({ queryKey: contractKeys.all }),
    qc.invalidateQueries({ queryKey: categoryKeys("contracts").all }),
  ])

export function useCategoryContracts(categoryId: string) {
  return useQuery({
    queryKey: contractKeys.byCategory(categoryId),
    queryFn: () => getContractsByCategory(categoryId),
  })
}

export function useContracts() {
  return useQuery({
    queryKey: contractKeys.lists(),
    queryFn: getAllContracts,
  })
}

export function useContract(id: string) {
  return useQuery({
    queryKey: contractKeys.detail(id),
    queryFn: () => getContractById(id),
  })
}

export function useUpcomingRenewals(days: number = 365) {
  return useQuery({
    queryKey: contractKeys.upcomingRenewals(days),
    queryFn: () => getUpcomingRenewals(days),
  })
}

export function useCreateContract(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: ContractFormData) => createContract(categoryId, data),
    onSuccess: () => invalidate(qc),
  })
}

export function useCreateContractByCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ categoryId, data }: { categoryId: string; data: ContractFormData }) => createContract(categoryId, data),
    onSuccess: () => invalidate(qc),
  })
}

export function useUpdateContract() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ContractFormData }) => updateContract(id, data),
    onSuccess: () => invalidate(qc),
  })
}

export function useDeleteContract() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteContract(id),
    onSuccess: () => invalidate(qc),
  })
}

export function useImportContracts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => importContracts(file),
    onSuccess: () => invalidate(qc),
  })
}
