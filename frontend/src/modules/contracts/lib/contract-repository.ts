import type { Contract, ContractFormData } from "@/modules/contracts/types"
import type { Summary } from "@/modules/contracts/types"
import { del, get, getToken, post, put } from "@/lib/api"

export interface ImportResult {
  created: number
  errors: { row: number; error: string }[]
}

export async function getAllContracts(): Promise<Contract[]> {
  return get<Contract[]>("/contracts")
}

export async function getContractsByCategory(categoryId: string): Promise<Contract[]> {
  return get<Contract[]>(`/categories/${categoryId}/contracts`)
}

export async function getContractById(id: string): Promise<Contract> {
  return get<Contract>(`/contracts/${id}`)
}

export async function createContract(categoryId: string, data: ContractFormData): Promise<Contract> {
  return post<Contract>(`/categories/${categoryId}/contracts`, data)
}

export async function updateContract(id: string, data: ContractFormData): Promise<Contract> {
  return put<Contract>(`/contracts/${id}`, data)
}

export async function deleteContract(id: string): Promise<void> {
  return del(`/contracts/${id}`)
}

export async function getUpcomingRenewals(days: number = 365): Promise<Contract[]> {
  return get<Contract[]>(`/contracts/upcoming-renewals?days=${days}`)
}

export async function getSummary(): Promise<Summary> {
  return get<Summary>("/contracts/summary")
}

export async function importContracts(file: File): Promise<ImportResult> {
  const form = new FormData()
  form.append("file", file)
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }
  const res = await fetch("/api/v1/contracts/import", {
    method: "POST",
    headers,
    body: form,
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error ?? res.statusText)
  }
  return res.json()
}
