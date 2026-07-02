import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
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

const contractsKey = (categoryId: string) => ["contracts", categoryId] as const

export function useCategoryContracts(categoryId: string) {
  return useQuery({
    queryKey: contractsKey(categoryId),
    queryFn: () => getContractsByCategory(categoryId),
  })
}

export function useContracts() {
  return useQuery({
    queryKey: ["contracts"],
    queryFn: getAllContracts,
  })
}

export function useContract(id: string) {
  return useQuery({
    queryKey: ["contracts", id],
    queryFn: () => getContractById(id),
  })
}

export function useUpcomingRenewals(days: number = 365) {
  return useQuery({
    queryKey: ["contracts", "upcoming-renewals", days],
    queryFn: () => getUpcomingRenewals(days),
  })
}

export function useCreateContract(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: ContractFormData) => createContract(categoryId, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: contractsKey(categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
    },
  })
}

export function useCreateContractByCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ categoryId, data }: { categoryId: string; data: ContractFormData }) => createContract(categoryId, data),
    onSuccess: (contract) => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: contractsKey(contract.categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
    },
  })
}

export function useUpdateContract(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ContractFormData }) => updateContract(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: contractsKey(categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
    },
  })
}

export function useUpdateContractById() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ContractFormData }) => updateContract(id, data),
    onSuccess: (contract) => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["contracts", contract.id] })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
    },
  })
}

export function useDeleteContract(categoryId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteContract(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: contractsKey(categoryId) })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
    },
  })
}

export function useImportContracts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => importContracts(file),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
    },
  })
}
