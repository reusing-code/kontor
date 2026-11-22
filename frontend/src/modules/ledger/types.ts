import { z } from "zod/v4"

// The balance is derived from the newest snapshot (asOf, source) plus the
// transactions booked after it; driftMinor compares that snapshot with the one
// before it plus the transactions in between (missing or duplicated rows).
export const ledgerAccountBalanceSchema = z.object({
  currentMinor: z.number().int(),
  snapshotMinor: z.number().int(),
  asOf: z.string(),
  source: z.string(),
  driftMinor: z.number().int().optional(),
  previousAsOf: z.string().optional(),
})

export type LedgerAccountBalance = z.infer<typeof ledgerAccountBalanceSchema>

export const ledgerAccountSchema = z.object({
  id: z.string().uuid(),
  name: z.string().min(1),
  bank: z.string().min(1),
  iban: z.string().optional(),
  currency: z.string().min(1),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
  balance: ledgerAccountBalanceSchema.optional(),
})

export type LedgerAccount = z.infer<typeof ledgerAccountSchema>

export const emailProcessedActions = ["keep", "move", "delete"] as const
export const emailProcessedActionSchema = z.enum(emailProcessedActions)
export type EmailProcessedAction = z.infer<typeof emailProcessedActionSchema>

export const ledgerEmailAccountSchema = z.object({
  id: z.string().uuid(),
  name: z.string().min(1),
  imapHost: z.string().min(1),
  imapPort: z.number().int(),
  username: z.string().min(1),
  useTls: z.boolean(),
  folder: z.string(),
  draftsFolder: z.string().optional(),
  // what happens in the mailbox to processed emails of folder / draftsFolder
  processedAction: emailProcessedActionSchema.optional(),
  processedFolder: z.string().optional(),
  draftsProcessedAction: emailProcessedActionSchema.optional(),
  draftsProcessedFolder: z.string().optional(),
  preClassify: z.boolean(),
  scanSince: z.string(),
  lastScanAt: z.string().datetime().optional(),
  lastScanStatusMessage: z.string().optional(),
  scanResumePending: z.boolean().optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type LedgerEmailAccount = z.infer<typeof ledgerEmailAccountSchema>

export const ledgerEmailAccountInputSchema = z.object({
  name: z.string().min(1),
  imapHost: z.string().min(1),
  imapPort: z.number().int().positive().default(993),
  username: z.string().min(1),
  // empty when editing: the stored password is kept
  password: z.string().optional(),
  useTls: z.boolean().default(true),
  folder: z.string().trim().min(1),
  // optional second folder whose emails are proposed in the draft inbox
  draftsFolder: z.string().trim().default(""),
  processedAction: emailProcessedActionSchema.default("keep"),
  processedFolder: z.string().trim().default(""),
  draftsProcessedAction: emailProcessedActionSchema.default("keep"),
  draftsProcessedFolder: z.string().trim().default(""),
  preClassify: z.boolean().default(false),
  scanSince: z.string().min(1),
}).refine((data) => !data.draftsFolder || data.draftsFolder !== data.folder, {
  path: ["draftsFolder"],
  message: "ledger.email.draftsFolderSame",
}).superRefine((data, ctx) => {
  // a move target is neither of the scanned folders; the drafts action only
  // counts while there is a drafts folder
  const targets = [
    { path: "processedFolder", active: data.processedAction === "move", folder: data.processedFolder },
    { path: "draftsProcessedFolder", active: data.draftsProcessedAction === "move" && !!data.draftsFolder, folder: data.draftsProcessedFolder },
  ]
  for (const target of targets) {
    if (!target.active) continue
    if (!target.folder) {
      ctx.addIssue({ code: "custom", path: [target.path], message: "ledger.email.processed.targetRequired" })
    } else if (target.folder === data.folder || target.folder === data.draftsFolder) {
      ctx.addIssue({ code: "custom", path: [target.path], message: "ledger.email.processed.targetSame" })
    }
  }
})

export type LedgerEmailAccountInput = z.infer<typeof ledgerEmailAccountInputSchema>

export const ledgerEmailOrderItemSchema = z.object({
  name: z.string().min(1),
  quantity: z.number().int(),
  unitPriceMinor: z.int(),
  totalPriceMinor: z.int(),
})

export type LedgerEmailOrderItem = z.infer<typeof ledgerEmailOrderItemSchema>

export const ledgerEmailOrderSchema = z.object({
  id: z.string().uuid(),
  emailAccountId: z.string().uuid(),
  importerId: z.string().min(1),
  externalOrderId: z.string().optional(),
  orderDate: z.string(),
  totalMinor: z.int(),
  currency: z.string().min(1),
  merchant: z.string().optional(),
  paymentMethod: z.string().optional(),
  items: z.array(ledgerEmailOrderItemSchema).optional(),
  emailMessageId: z.string().optional(),
  emailSubject: z.string().optional(),
  matchStatus: z.enum(["unmatched", "matched", "rejected"]),
  linkedTransactionIds: z.array(z.string().uuid()).optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type LedgerEmailOrder = z.infer<typeof ledgerEmailOrderSchema>

export const ledgerEmailOrderLinkInputSchema = z.object({
  transactionIds: z.array(z.string().uuid()).min(1),
})

export type LedgerEmailOrderLinkInput = z.infer<typeof ledgerEmailOrderLinkInputSchema>

export const ledgerEmailScanResultSchema = z.object({
  emailsScanned: z.number().int(),
  ordersFound: z.number().int(),
  ordersNew: z.number().int(),
  ordersLinked: z.number().int(),
  warnings: z.array(z.string()).optional(),
  orders: z.array(ledgerEmailOrderSchema).optional(),
  partial: z.boolean().optional(),
  skipped: z.number().int().optional(),
})

export type LedgerEmailScanResult = z.infer<typeof ledgerEmailScanResultSchema>

export interface LedgerEmailDraftRunResult {
  found: number
  created: number
  skipped: number
  failed: number
  warnings: string[]
}

export type LedgerEmailLinkEntityType = "contract" | "purchase" | "cost"

// The email an entity was created from (through the draft inbox).
export interface LedgerEmailLink {
  entityType: LedgerEmailLinkEntityType
  entityId: string
  emailAccountId: string
  accountName: string
  messageId?: string
  subject?: string
  from?: string
  date?: string
  createdAt: string
}

export const ledgerEmailScanStatusSchema = z.object({
  state: z.enum(["idle", "running", "done", "partial", "failed"]),
  trigger: z.enum(["manual", "scheduled", "upload"]).optional(),
  startedAt: z.string().datetime().optional(),
  finishedAt: z.string().datetime().optional(),
  emailsScanned: z.number().int(),
  ordersNew: z.number().int(),
  ordersLinked: z.number().int(),
  skipped: z.number().int(),
  warnings: z.array(z.string()),
  error: z.string().optional(),
  resumes: z.boolean().optional(),
})

export type LedgerEmailScanStatus = z.infer<typeof ledgerEmailScanStatusSchema>

export const ledgerCategorySchema = z.object({
  id: z.string().uuid(),
  name: z.string().min(1),
  parentId: z.string().uuid().optional(),
  matchWords: z.array(z.string()),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type LedgerCategory = z.infer<typeof ledgerCategorySchema>

export const ledgerCategoryInputSchema = z.object({
  name: z.string().min(1),
  parentId: z.string().uuid().optional(),
  matchWords: z.array(z.string()).default([]),
})

export type LedgerCategoryInput = z.infer<typeof ledgerCategoryInputSchema>

// A part of a transaction booked to another category; the rest of the amount
// keeps the transaction's own category. categoryId is only missing after the
// category was deleted.
export const ledgerTransactionSplitSchema = z.object({
  amountMinor: z.int(),
  categoryId: z.string().uuid().optional(),
  note: z.string().optional(),
})

export type LedgerTransactionSplit = z.infer<typeof ledgerTransactionSplitSchema>

export const ledgerTransactionSchema = z.object({
  id: z.string().uuid(),
  accountId: z.string().uuid(),
  categoryId: z.string().uuid().optional(),
  splits: z.array(ledgerTransactionSplitSchema).optional(),
  specialCategory: z.string().optional(),
  transferPairTransactionId: z.string().uuid().optional(),
  bookingDate: z.string(),
  valueDate: z.string().optional(),
  amountMinor: z.int(),
  currency: z.string(),
  counterpartyName: z.string().optional(),
  counterpartyIban: z.string().optional(),
  purpose: z.string().optional(),
  bankReference: z.string().optional(),
  transactionType: z.string().optional(),
  reviewStatus: z.string(),
  categorizationSource: z.string(),
  note: z.string().optional(),
  links: z.array(z.string().url()).optional(),
  references: z.array(z.object({
    type: z.enum(["purchase", "contract", "vehicle", "cost"]),
    targetId: z.string().uuid(),
  })).optional(),
  emailOrderIds: z.array(z.string().uuid()).optional(),
  sourceType: z.string(),
  importBatchId: z.string().uuid(),
  fingerprint: z.string(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
})

export type LedgerTransaction = z.infer<typeof ledgerTransactionSchema>

export const ledgerTransferCandidateSchema = z.object({
  transaction: ledgerTransactionSchema,
  accountName: z.string(),
  dateDeltaDays: z.number().int(),
  ibanMatch: z.boolean(),
})

export type LedgerTransferCandidate = z.infer<typeof ledgerTransferCandidateSchema>

export const ledgerTransferCandidatesResultSchema = z.object({
  items: z.array(ledgerTransferCandidateSchema),
})

export type LedgerTransferCandidatesResult = z.infer<typeof ledgerTransferCandidatesResultSchema>

export const ledgerEmailOrderCandidateSchema = z.object({
  transaction: ledgerTransactionSchema,
  accountName: z.string(),
  dateDeltaDays: z.number().int().optional(),
  amountComparable: z.boolean(),
})

export type LedgerEmailOrderCandidate = z.infer<typeof ledgerEmailOrderCandidateSchema>

export const ledgerEmailOrderConversionSchema = z.object({
  currency: z.string(),
  totalMinor: z.number().int(),
  rate: z.number(),
  rateDate: z.string(),
})

export type LedgerEmailOrderConversion = z.infer<typeof ledgerEmailOrderConversionSchema>

export const ledgerEmailOrderCandidatesResultSchema = z.object({
  items: z.array(ledgerEmailOrderCandidateSchema),
  beforeDays: z.number().int(),
  afterDays: z.number().int(),
  conversions: z.array(ledgerEmailOrderConversionSchema).optional(),
  unconvertedCurrencies: z.array(z.string()).optional(),
})

export type LedgerEmailOrderCandidatesResult = z.infer<typeof ledgerEmailOrderCandidatesResultSchema>

export const ledgerTransferLinkInputSchema = z.object({
  pairedTransactionId: z.string().uuid(),
})

export type LedgerTransferLinkInput = z.infer<typeof ledgerTransferLinkInputSchema>

export const ledgerTransferLinkResultSchema = z.object({
  transaction: ledgerTransactionSchema,
  pairedTransaction: ledgerTransactionSchema.optional(),
})

export type LedgerTransferLinkResult = z.infer<typeof ledgerTransferLinkResultSchema>

export const ledgerTransactionReferenceSchema = z.object({
  type: z.enum(["purchase", "contract", "vehicle", "cost"]),
  targetId: z.string().uuid(),
})

export type LedgerTransactionReference = z.infer<typeof ledgerTransactionReferenceSchema>

export const ledgerTransactionDetailsInputSchema = z.object({
  note: z.string().optional(),
  links: z.array(z.string().url()).default([]),
  references: z.array(ledgerTransactionReferenceSchema).default([]),
})

export type LedgerTransactionDetailsInput = z.infer<typeof ledgerTransactionDetailsInputSchema>

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
  suggestedCategoryId: z.string().uuid().optional(),
  suggestedCategoryName: z.string().optional(),
  reviewStatus: z.string(),
  categorizationSource: z.string(),
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
  balance: z.object({ amountMinor: z.number().int(), asOf: z.string() }).optional(),
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

// closingMinor is the balance after the page's newest transaction (top),
// openingMinor the balance before its oldest one (bottom).
export const ledgerPageBalanceSchema = z.object({
  closingMinor: z.number().int(),
  openingMinor: z.number().int(),
})

export type LedgerPageBalance = z.infer<typeof ledgerPageBalanceSchema>

export const ledgerTransactionsPageSchema = z.object({
  items: z.array(ledgerTransactionSchema),
  nextCursor: z.string().optional(),
  pageBalance: ledgerPageBalanceSchema.optional(),
  accountBalance: ledgerAccountBalanceSchema.optional(),
})

export type LedgerTransactionsPage = z.infer<typeof ledgerTransactionsPageSchema>

export const ledgerSuggestionConfidences = ["high", "medium", "low"] as const
export type LedgerSuggestionConfidence = (typeof ledgerSuggestionConfidences)[number]

export const ledgerSuggestionSchema = z.object({
  categoryId: z.string().uuid(),
  confidence: z.enum(ledgerSuggestionConfidences),
  matchWord: z.string().optional(),
})

export type LedgerSuggestion = z.infer<typeof ledgerSuggestionSchema>

export const ledgerReviewQueueItemSchema = ledgerTransactionSchema.extend({
  suggestion: ledgerSuggestionSchema.optional(),
})

export type LedgerReviewQueueItem = z.infer<typeof ledgerReviewQueueItemSchema>

export const ledgerReviewQueuePageSchema = z.object({
  items: z.array(ledgerReviewQueueItemSchema),
  nextCursor: z.string().optional(),
})

export type LedgerReviewQueuePage = z.infer<typeof ledgerReviewQueuePageSchema>

export const ledgerSuggestionRunResultSchema = z.object({
  candidates: z.int(),
  ruleMatched: z.int(),
  suggested: z.int(),
  noSuggestion: z.int(),
  remaining: z.int(),
  partial: z.boolean(),
  warnings: z.array(z.string()),
})

export type LedgerSuggestionRunResult = z.infer<typeof ledgerSuggestionRunResultSchema>

export interface LedgerRecurringRunResult {
  groups: number
  created: number
  skipped: number
}

export interface LedgerSuggestionStatus {
  running: boolean
}

export interface LedgerBatchReviewItem {
  transactionId: string
  categoryId: string
  addMatchWords?: string[]
}

export type LedgerBatchReviewFailureCode = "not_found" | "transfer_linked" | "internal_error"

export interface LedgerBatchReviewResult {
  reviewed: number
  failed: Array<{ transactionId: string; code: LedgerBatchReviewFailureCode }>
}

export const ledgerReviewResultSchema = z.object({
  transaction: ledgerTransactionSchema,
  category: ledgerCategorySchema.optional(),
})

export type LedgerReviewResult = z.infer<typeof ledgerReviewResultSchema>

export const ledgerReviewInputSchema = z.object({
  categoryId: z.string().uuid().optional(),
  newCategory: ledgerCategoryInputSchema.optional(),
  addMatchWords: z.array(z.string()).default([]),
}).refine((value) => !(value.categoryId && value.newCategory), {
  message: "categoryId and newCategory are mutually exclusive",
})

export type LedgerReviewInput = z.infer<typeof ledgerReviewInputSchema>

export const ledgerAccountInputSchema = z.object({
  name: z.string().min(1),
  bank: z.string().min(1),
  iban: z.string().optional(),
  currency: z.string().min(1).default("EUR"),
})

export type LedgerAccountInput = z.infer<typeof ledgerAccountInputSchema>

// What editing an account may change; IBAN and currency are fixed.
export const ledgerAccountUpdateInputSchema = z.object({
  name: z.string().trim().min(1).max(200),
  bank: z.string().trim().min(1).max(200),
})

export type LedgerAccountUpdateInput = z.infer<typeof ledgerAccountUpdateInputSchema>

// What deleting an account removes (GET /ledger/accounts/{id}/delete-summary).
export interface LedgerAccountDeleteSummary {
  transactions: number
  imports: number
  references: number
  transfers: number
  emailOrders: number
  bankAccounts: Array<{ connectionId: string; bank: string; uid: string; name: string; iban: string }>
}

export const ledgerCommitRequestSchema = z.object({
  accountId: z.string().uuid().optional(),
  newAccount: ledgerAccountInputSchema.optional(),
}).refine((value) => !(value.accountId && value.newAccount), {
  message: "accountId and newAccount are mutually exclusive",
})

export type LedgerCommitRequest = z.infer<typeof ledgerCommitRequestSchema>

export type LedgerSourceType = "dkb.csv" | "comdirect.csv"

export type LedgerReviewStatus = "needsReview" | "confirmed"
export type LedgerCategorizationSource = "none" | "keyword" | "manual"

export interface BankInfo {
  name: string
  country: string
  logo: string
  verified: boolean
  maxConsentDays: number
}

export type BankConnectionStatus = "active" | "expiring" | "expired"

export interface BankAccount {
  uid: string
  iban: string
  currency: string
  name: string
  ledgerAccountId?: string
  mapped: boolean
  lastSyncAt?: string
  lastSyncAttemptAt?: string
  lastSyncStatus: string
  statusMessage: string
  lastImported: number
  lastDuplicates: number
  lastBalanceStatus?: "ok" | "unavailable" | "error"
}

export interface BankConnection {
  id: string
  aspspName: string
  aspspCountry: string
  authorizedAt: string
  expiresAt: string
  status: BankConnectionStatus
  // false for a consent from before balances were requested
  balancesRequested: boolean
  accounts: BankAccount[]
}

export interface BankAuthorizeInput {
  aspspName: string
  aspspCountry: string
}

export interface BankSyncAccountResult {
  uid: string
  status: string
  imported: number
  duplicates: number
  skippedNotBooked: number
  skippedCurrency: number
  skippedInvalid: number
}

export interface BankSyncSummary {
  imported: number
  duplicates: number
  accounts: BankSyncAccountResult[]
}

export interface LedgerManualBalanceInput {
  amountMinor: number
  asOf: string
}

// An inclusive booking-date range of the analyses; an omitted end is left to
// the server (from = the first booking, to = today).
export interface LedgerAnalysisRange {
  from?: string
  to?: string
}

export interface LedgerSpendingSeries {
  kind: "category" | "uncategorized" | "direct"
  categoryId?: string
  name: string
  amountsMinor: number[]
  totalMinor: number
}

// Monthly spending per series; with parentCategoryId the amounts are signed.
export interface LedgerSpendingAnalysis {
  from: string
  to: string
  months: string[]
  parentCategoryId?: string
  series: LedgerSpendingSeries[]
  totalsMinor: number[]
  excludedCurrencies: string[]
}
