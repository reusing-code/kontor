import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  getAllVehicles,
  getVehicleById,
  createVehicle,
  updateVehicle,
  deleteVehicle,
  getVehicleSummary,
  getCostEntries,
  createCostEntry,
  updateCostEntry,
  deleteCostEntry,
} from "@/modules/auto/lib/vehicle-repository"
import type { VehicleFormData, CostEntryFormData } from "@/modules/auto/types"

export function useVehicles() {
  return useQuery({
    queryKey: ["vehicles"],
    queryFn: getAllVehicles,
  })
}

export function useVehicle(id: string) {
  return useQuery({
    queryKey: ["vehicles", id],
    queryFn: () => getVehicleById(id),
  })
}

export function useCreateVehicle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: VehicleFormData) => createVehicle(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vehicles"] })
    },
  })
}

export function useUpdateVehicle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: VehicleFormData }) => updateVehicle(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vehicles"] })
    },
  })
}

export function useDeleteVehicle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteVehicle(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vehicles"] })
    },
  })
}

export function useVehicleSummary(vehicleId: string) {
  return useQuery({
    queryKey: ["vehicles", vehicleId, "summary"],
    queryFn: () => getVehicleSummary(vehicleId),
  })
}

export function useCostEntries(vehicleId: string) {
  return useQuery({
    queryKey: ["vehicles", vehicleId, "costs"],
    queryFn: () => getCostEntries(vehicleId),
  })
}

export function useCreateCostEntry(vehicleId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CostEntryFormData) => createCostEntry(vehicleId, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vehicles", vehicleId, "costs"] })
      qc.invalidateQueries({ queryKey: ["vehicles", vehicleId, "summary"] })
    },
  })
}

export function useUpdateCostEntry(vehicleId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: CostEntryFormData }) => updateCostEntry(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vehicles", vehicleId, "costs"] })
      qc.invalidateQueries({ queryKey: ["vehicles", vehicleId, "summary"] })
    },
  })
}

export function useDeleteCostEntry(vehicleId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteCostEntry(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vehicles", vehicleId, "costs"] })
      qc.invalidateQueries({ queryKey: ["vehicles", vehicleId, "summary"] })
    },
  })
}
