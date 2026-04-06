import { z } from "zod/v4"

export const ledgerAccountSchema = z.object({
  id: z.string().uuid(),
  name: z.string().min(1),
  bank: z.string().min(1),
  iban: z.string().optional(),
  currency: z.string().min(1),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type LedgerAccount = z.infer<typeof ledgerAccountSchema>

export const ledgerTransactionSchema = z.object({
  id: z.string().uuid(),
  accountId: z.string().uuid(),
  bookingDate: z.string(),
  valueDate: z.string().optional(),
  amountMinor: z.int(),
  currency: z.string(),
  counterpartyName: z.string().optional(),
  counterpartyIban: z.string().optional(),
  purpose: z.string().optional(),
  bankReference: z.string().optional(),
  transactionType: z.string().optional(),
  sourceType: z.string(),
  importBatchId: z.string().uuid(),
  fingerprint: z.string(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type LedgerTransaction = z.infer<typeof ledgerTransactionSchema>

export const ledgerImportBatchSchema = z.object({
  id: z.string().uuid(),
  accountId: z.string().uuid(),
  sourceType: z.string(),
  parserVersion: z.string(),
  filename: z.string(),
  fileSha256: z.string(),
  status: z.string(),
  totalRows: z.number().int(),
  importedRows: z.number().int(),
  duplicateRows: z.number().int(),
  errorRows: z.number().int(),
  warnings: z.array(z.string()).optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type LedgerImportBatch = z.infer<typeof ledgerImportBatchSchema>

export const ledgerPreviewRowSchema = z.object({
  bookingDate: z.string(),
  valueDate: z.string().optional(),
  amountMinor: z.int(),
  currency: z.string(),
  counterpartyName: z.string().optional(),
  counterpartyIban: z.string().optional(),
  purpose: z.string().optional(),
  bankReference: z.string().optional(),
  transactionType: z.string().optional(),
})

export type LedgerPreviewRow = z.infer<typeof ledgerPreviewRowSchema>

export const ledgerPreviewTransactionSchema = z.object({
  row: ledgerPreviewRowSchema,
  fingerprint: z.string(),
  isDuplicate: z.boolean(),
})

export type LedgerPreviewTransaction = z.infer<typeof ledgerPreviewTransactionSchema>

export const ledgerPreviewResultSchema = z.object({
  previewId: z.string(),
  sourceType: z.string(),
  filename: z.string(),
  fileSha256: z.string(),
  accountId: z.string().optional(),
  iban: z.string().optional(),
  bankName: z.string().optional(),
  transactions: z.array(ledgerPreviewTransactionSchema),
  totalRows: z.number().int(),
  newRows: z.number().int(),
  duplicateRows: z.number().int(),
  warnings: z.array(z.string()).optional(),
  expiresAt: z.string().datetime(),
})

export type LedgerPreviewResult = z.infer<typeof ledgerPreviewResultSchema>

export const ledgerCommitResultSchema = z.object({
  batchId: z.string().uuid(),
  accountId: z.string().uuid(),
  importedRows: z.number().int(),
  duplicateRows: z.number().int(),
})

export type LedgerCommitResult = z.infer<typeof ledgerCommitResultSchema>

export const ledgerTransactionsPageSchema = z.object({
  items: z.array(ledgerTransactionSchema),
  nextCursor: z.string().optional(),
})

export type LedgerTransactionsPage = z.infer<typeof ledgerTransactionsPageSchema>

export const ledgerAccountInputSchema = z.object({
  name: z.string().min(1),
  bank: z.string().min(1),
  iban: z.string().optional(),
  currency: z.string().min(1).default("EUR"),
})

export type LedgerAccountInput = z.infer<typeof ledgerAccountInputSchema>

export const ledgerCommitRequestSchema = z.object({
  accountId: z.string().uuid().optional(),
  newAccount: ledgerAccountInputSchema.optional(),
}).refine((value) => !(value.accountId && value.newAccount), {
  message: "accountId and newAccount are mutually exclusive",
})

export type LedgerCommitRequest = z.infer<typeof ledgerCommitRequestSchema>

export type LedgerSourceType = "dkb.csv" | "comdirect.csv"
