const all = ["vehicles"] as const

export const vehicleKeys = {
  all,
  list: () => [...all, "list"] as const,
  detail: (id: string) => [...all, "detail", id] as const,
  summary: (vehicleId: string) => [...all, "detail", vehicleId, "summary"] as const,
  costs: (vehicleId: string) => [...all, "detail", vehicleId, "costs"] as const,
  cost: (id: string) => [...all, "costs", id] as const,
}
