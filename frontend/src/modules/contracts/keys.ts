import { categoryItemKeys } from "@/lib/query-keys"

const base = categoryItemKeys("contracts")

export const contractKeys = {
  ...base,
  upcomingRenewals: (days: number) => [...base.all, "upcoming-renewals", days] as const,
}
