import { z } from "zod/v4"
import { numericString } from "@/lib/utils"

export const costTypeSchema = z.enum([
  "service",
  "fuel",
  "insurance",
  "tax",
  "inspection",
  "tires",
  "mileage",
  "misc",
])

export type CostType = z.infer<typeof costTypeSchema>

export const vehicleSchema = z.object({
  id: z.string().uuid(),
  linkedTransactionIds: z.array(z.string().uuid()).optional(),
  name: z.string().min(1),
  make: z.string().optional(),
  model: z.string().optional(),
  year: z.number().int().optional(),
  licensePlate: z.string().optional(),
  purchaseDate: z.string().date().optional(),
  purchasePrice: z.number().nonnegative().optional(),
  purchaseMileage: z.number().nonnegative().optional(),
  targetMileage: z.number().nonnegative().optional(),
  targetMonths: z.number().int().nonnegative().optional(),
  annualInsurance: z.number().nonnegative().optional(),
  annualTax: z.number().nonnegative().optional(),
  maintenanceFactor: z.number().nonnegative().optional(),
  comments: z.string().optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type Vehicle = z.infer<typeof vehicleSchema>

export const vehicleFormSchema = z.object({
  name: z.string().min(1),
  make: z.string().optional(),
  model: z.string().optional(),
  year: numericString(z.number().int()),
  licensePlate: z.string().optional(),
  purchaseDate: z.string().date().optional(),
  purchasePrice: numericString(z.number().nonnegative()),
  purchaseMileage: numericString(z.number().nonnegative()),
  targetMileage: numericString(z.number().nonnegative()),
  targetMonths: numericString(z.number().int().nonnegative()),
  annualInsurance: numericString(z.number().nonnegative()),
  annualTax: numericString(z.number().nonnegative()),
  maintenanceFactor: numericString(z.number().nonnegative()),
  comments: z.string().optional(),
})

export type VehicleFormData = z.infer<typeof vehicleFormSchema>

export const costEntrySchema = z.object({
  id: z.string().uuid(),
  vehicleId: z.string().uuid(),
  type: costTypeSchema,
  description: z.string().optional(),
  vendor: z.string().optional(),
  amount: z.number().nonnegative().optional(),
  date: z.string().date(),
  mileage: z.number().nonnegative().optional(),
  comments: z.string().optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type CostEntry = z.infer<typeof costEntrySchema>

export const costEntryFormSchema = z.object({
  type: costTypeSchema,
  description: z.string().optional(),
  vendor: z.string().optional(),
  amount: numericString(z.number().nonnegative()),
  date: z.string().date(),
  mileage: numericString(z.number().nonnegative()),
  comments: z.string().optional(),
})

export type CostEntryFormData = z.infer<typeof costEntryFormSchema>

export interface YearCosts {
  year: number
  service: number
  fuel: number
  insurance: number
  tax: number
  inspection: number
  tires: number
  misc: number
  total: number
}

export interface MileagePoint {
  date: string
  mileage: number
}

export interface YearMileage {
  year: number
  km: number
  fuelCostPerKm: number
}

export interface VehicleProjection {
  targetMileage?: number
  targetMonths?: number
  projectedTotalCost: number
  projectedCostPerMonth: number
  projectedCostPerKm: number
  theoreticalResidualValue: number
  requiredSalePrice: number
}

export interface VehicleSummary {
  vehicle: Vehicle
  currentMileage: number
  monthsOwned: number
  kmPerMonth: number
  costsByType: Record<string, number>
  costsByYear: YearCosts[]
  totalCost: number
  costPerMonth: number
  costPerKm: number
  projection?: VehicleProjection
  mileageByYear: YearMileage[]
  mileageHistory: MileagePoint[]
  entryCount: number
}
