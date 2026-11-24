import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  getAllVehicles,
  getVehicleById,
  createVehicle,
  updateVehicle,
  deleteVehicle,
  getVehicleSummary,
  getCostEntries,
  getCostEntryById,
  createCostEntry,
  updateCostEntry,
  deleteCostEntry,
} from "@/modules/auto/lib/vehicle-repository"
import { vehicleKeys } from "@/modules/auto/keys"
import type { VehicleFormData, CostEntryFormData } from "@/modules/auto/types"

export function useVehicles() {
  return useQuery({
    queryKey: vehicleKeys.list(),
    queryFn: getAllVehicles,
  })
}

export function useVehicle(id: string) {
  return useQuery({
    queryKey: vehicleKeys.detail(id),
    queryFn: () => getVehicleById(id),
  })
}

export function useCreateVehicle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: VehicleFormData) => createVehicle(data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: vehicleKeys.all })
    },
  })
}

export function useUpdateVehicle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: VehicleFormData }) => updateVehicle(id, data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: vehicleKeys.all })
    },
  })
}

export function useDeleteVehicle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteVehicle(id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: vehicleKeys.all })
    },
  })
}

export function useVehicleSummary(vehicleId: string) {
  return useQuery({
    queryKey: vehicleKeys.summary(vehicleId),
    queryFn: () => getVehicleSummary(vehicleId),
  })
}

export function useCostEntries(vehicleId: string) {
  return useQuery({
    queryKey: vehicleKeys.costs(vehicleId),
    queryFn: () => getCostEntries(vehicleId),
  })
}

export function useCostEntry(id: string) {
  return useQuery({
    queryKey: vehicleKeys.cost(id),
    queryFn: () => getCostEntryById(id),
  })
}

export function useCreateCostEntry(vehicleId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CostEntryFormData) => createCostEntry(vehicleId, data),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: vehicleKeys.costs(vehicleId) }),
        qc.invalidateQueries({ queryKey: vehicleKeys.summary(vehicleId) }),
      ])
    },
  })
}

export function useCreateCostEntryByVehicle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ vehicleId, data }: { vehicleId: string; data: CostEntryFormData }) => createCostEntry(vehicleId, data),
    onSuccess: async (entry) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: vehicleKeys.costs(entry.vehicleId) }),
        qc.invalidateQueries({ queryKey: vehicleKeys.summary(entry.vehicleId) }),
      ])
    },
  })
}

export function useUpdateCostEntry(vehicleId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: CostEntryFormData }) => updateCostEntry(id, data),
    onSuccess: async (entry) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: vehicleKeys.costs(vehicleId) }),
        qc.invalidateQueries({ queryKey: vehicleKeys.summary(vehicleId) }),
        qc.invalidateQueries({ queryKey: vehicleKeys.cost(entry.id) }),
      ])
    },
  })
}

export function useDeleteCostEntry(vehicleId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteCostEntry(id),
    onSuccess: async (_, id) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: vehicleKeys.costs(vehicleId) }),
        qc.invalidateQueries({ queryKey: vehicleKeys.summary(vehicleId) }),
        qc.invalidateQueries({ queryKey: vehicleKeys.cost(id) }),
      ])
    },
  })
}
