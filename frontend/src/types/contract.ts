import { z } from "zod/v4"
import { numericString } from "@/lib/utils"

export const billingIntervalSchema = z.enum(["monthly", "yearly"])
export type BillingInterval = z.infer<typeof billingIntervalSchema>

export const contractSchema = z.object({
  id: z.string().uuid(),
  categoryId: z.string().uuid(),
  name: z.string().min(1),
  productName: z.string().optional(),
  company: z.string().optional(),
  contractNumber: z.string().optional(),
  customerNumber: z.string().optional(),
  price: z.number().nonnegative().optional(),
  billingInterval: billingIntervalSchema,
  startDate: z.string().date(),
  endDate: z.string().date().optional(),
  minimumDurationMonths: z.number().int().nonnegative(),
  extensionDurationMonths: z.number().int().nonnegative(),
  noticePeriodMonths: z.number().int().nonnegative(),
  customerPortalUrl: z.string().url().optional().or(z.literal("")),
  paperlessUrl: z.string().url().optional().or(z.literal("")),
  comments: z.string().optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
  cancellationDate: z.string().date().optional(),
  expired: z.boolean().optional(),
})

export type Contract = z.infer<typeof contractSchema>

export const contractFormSchema = z.object({
  name: z.string().min(1),
  productName: z.string().optional(),
  company: z.string().optional(),
  contractNumber: z.string().optional(),
  customerNumber: z.string().optional(),
  price: numericString(z.number().nonnegative()),
  billingInterval: billingIntervalSchema,
  startDate: z.string().date(),
  endDate: z.string().date().optional(),
  minimumDurationMonths: numericString(z.number().int().nonnegative()),
  extensionDurationMonths: numericString(z.number().int().nonnegative()),
  noticePeriodMonths: numericString(z.number().int().nonnegative()),
  customerPortalUrl: z.string().url().optional().or(z.literal("")),
  paperlessUrl: z.string().url().optional().or(z.literal("")),
  comments: z.string().optional(),
})

export type ContractFormData = z.infer<typeof contractFormSchema>
