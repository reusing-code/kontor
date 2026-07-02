import type { Vehicle, VehicleFormData, CostEntry, CostEntryFormData, VehicleSummary } from "@/modules/auto/types"
import { del, get, post, put } from "@/lib/api"

export async function getAllVehicles(): Promise<Vehicle[]> {
  return get<Vehicle[]>("/vehicles")
}

export async function getVehicleById(id: string): Promise<Vehicle> {
  return get<Vehicle>(`/vehicles/${id}`)
}

export async function createVehicle(data: VehicleFormData): Promise<Vehicle> {
  return post<Vehicle>("/vehicles", data)
}

export async function updateVehicle(id: string, data: VehicleFormData): Promise<Vehicle> {
  return put<Vehicle>(`/vehicles/${id}`, data)
}

export async function deleteVehicle(id: string): Promise<void> {
  return del(`/vehicles/${id}`)
}

export async function getVehicleSummary(id: string): Promise<VehicleSummary> {
  return get<VehicleSummary>(`/vehicles/${id}/summary`)
}

export async function getCostEntries(vehicleId: string): Promise<CostEntry[]> {
  return get<CostEntry[]>(`/vehicles/${vehicleId}/costs`)
}

export async function getCostEntryById(id: string): Promise<CostEntry> {
  return get<CostEntry>(`/costs/${id}`)
}

export async function createCostEntry(vehicleId: string, data: CostEntryFormData): Promise<CostEntry> {
  return post<CostEntry>(`/vehicles/${vehicleId}/costs`, data)
}

export async function updateCostEntry(id: string, data: CostEntryFormData): Promise<CostEntry> {
  return put<CostEntry>(`/costs/${id}`, data)
}

export async function deleteCostEntry(id: string): Promise<void> {
  return del(`/costs/${id}`)
}
