import { z } from "zod/v4"
import { numericString } from "@/lib/utils"

export const purchaseSchema = z.object({
  id: z.string().uuid(),
  categoryId: z.string().uuid(),
  type: z.string().optional(),
  itemName: z.string().min(1),
  brand: z.string().optional(),
  articleNumber: z.string().optional(),
  dealer: z.string().optional(),
  price: z.number().nonnegative().optional(),
  purchaseDate: z.string().date().optional(),
  descriptionUrl: z.string().url().optional().or(z.literal("")),
  invoiceUrl: z.string().url().optional().or(z.literal("")),
  handbookUrl: z.string().url().optional().or(z.literal("")),
  consumables: z.string().optional(),
  comments: z.string().optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type Purchase = z.infer<typeof purchaseSchema>

export const purchaseFormSchema = z.object({
  type: z.string().optional(),
  itemName: z.string().min(1),
  brand: z.string().optional(),
  articleNumber: z.string().optional(),
  dealer: z.string().optional(),
  price: numericString(z.number().nonnegative()),
  purchaseDate: z.string().date().optional(),
  descriptionUrl: z.string().url().optional().or(z.literal("")),
  invoiceUrl: z.string().url().optional().or(z.literal("")),
  handbookUrl: z.string().url().optional().or(z.literal("")),
  consumables: z.string().optional(),
  comments: z.string().optional(),
})

export type PurchaseFormData = z.infer<typeof purchaseFormSchema>

export interface PurchaseCategorySummary {
  id: string
  name: string
  purchaseCount: number
  totalSpent: number
}

export interface PurchaseSummary {
  totalPurchases: number
  totalSpent: number
  categories: PurchaseCategorySummary[]
}
