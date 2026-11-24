import { z } from "zod/v4"

// The modules whose items are filed under the shared categories.
export type CategoryKind = "contracts" | "purchases"

export const categorySchema = z.object({
  id: z.string().uuid(),
  name: z.string().min(1),
  nameKey: z.string().optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type Category = z.infer<typeof categorySchema>

export const categoryFormSchema = z.object({
  name: z.string().min(1),
})

export type CategoryFormData = z.infer<typeof categoryFormSchema>
