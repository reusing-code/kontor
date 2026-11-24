import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { createCategoryItemHooks } from "@/hooks/category-item-hooks"
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
import type { Contract, ContractFormData } from "@/modules/contracts/types"

export const contractHooks = createCategoryItemHooks<Contract, ContractFormData>({
  kind: "contracts",
  keys: contractKeys,
  repository: {
    getAll: getAllContracts,
    getByCategory: getContractsByCategory,
    getById: getContractById,
    create: createContract,
    update: updateContract,
    remove: deleteContract,
  },
})

export const useContracts = contractHooks.useItems
export const useCategoryContracts = contractHooks.useItemsByCategory
export const useContract = contractHooks.useItem
export const useCreateContract = contractHooks.useCreateItem
export const useCreateContractByCategory = contractHooks.useCreateItemByCategory
export const useUpdateContract = contractHooks.useUpdateItem
export const useDeleteContract = contractHooks.useDeleteItem

export function useUpcomingRenewals(days: number = 365) {
  return useQuery({
    queryKey: contractKeys.upcomingRenewals(days),
    queryFn: () => getUpcomingRenewals(days),
  })
}

export function useImportContracts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => importContracts(file),
    onSuccess: () => contractHooks.invalidate(qc),
  })
}
