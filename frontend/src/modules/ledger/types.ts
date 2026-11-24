import { z } from "zod/v4"

// API response shapes are plain types (they mirror the Go structs: a field
// with omitempty is optional); zod schemas exist only for the form inputs that
// are validated at runtime.

export type LedgerSourceType = "dkb.csv" | "comdirect.csv" | "enablebanking"
// what a file import can be parsed as; enablebanking comes only from a bank sync
export type LedgerFileSourceType = Exclude<LedgerSourceType, "enablebanking">

export type LedgerReviewStatus = "needsReview" | "confirmed"
export type LedgerCategorizationSource = "none" | "keyword" | "manual"

// The balance is derived from the newest snapshot (asOf, source) plus the
// transactions booked after it; driftMinor compares that snapshot with the one
// before it plus the transactions in between (missing or duplicated rows).
export interface LedgerAccountBalance {
  currentMinor: number
  snapshotMinor: number
  asOf: string
  // a LedgerSourceType or "manual"
  source: string
  driftMinor?: number
  previousAsOf?: string
}

export interface LedgerAccount {
  id: string
  name: string
  bank: string
  iban?: string
  currency: string
  createdAt: string
  updatedAt: string
  balance?: LedgerAccountBalance
}

export const emailProcessedActions = ["keep", "move", "delete"] as const
export const emailProcessedActionSchema = z.enum(emailProcessedActions)
export type EmailProcessedAction = (typeof emailProcessedActions)[number]

export interface LedgerEmailAccount {
  id: string
  name: string
  imapHost: string
  imapPort: number
  username: string
  useTls: boolean
  folder: string
  draftsFolder?: string
  // what happens in the mailbox to processed emails of folder / draftsFolder
  processedAction?: EmailProcessedAction
  processedFolder?: string
  draftsProcessedAction?: EmailProcessedAction
  draftsProcessedFolder?: string
  preClassify: boolean
  scanSince: string
  lastScanAt?: string
  lastScanStatusMessage?: string
  scanResumePending?: boolean
  createdAt: string
  updatedAt: string
}

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

export interface LedgerEmailOrderItem {
  name: string
  quantity: number
  unitPriceMinor: number
  totalPriceMinor: number
}

export type LedgerEmailOrderMatchStatus = "unmatched" | "matched" | "rejected"

export interface LedgerEmailOrder {
  id: string
  emailAccountId: string
  importerId: string
  externalOrderId?: string
  orderDate: string
  totalMinor: number
  currency: string
  merchant?: string
  paymentMethod?: string
  items?: LedgerEmailOrderItem[]
  emailMessageId?: string
  emailSubject?: string
  matchStatus: LedgerEmailOrderMatchStatus
  linkedTransactionIds?: string[]
  createdAt: string
  updatedAt: string
}

export interface LedgerEmailOrderLinkInput {
  transactionIds: string[]
}

export interface LedgerEmailScanResult {
  emailsScanned: number
  ordersFound: number
  ordersNew: number
  ordersLinked: number
  warnings?: string[]
  orders?: LedgerEmailOrder[]
  partial?: boolean
  skipped?: number
}

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

export interface LedgerEmailScanStatus {
  state: "idle" | "running" | "done" | "partial" | "failed"
  trigger?: "manual" | "scheduled" | "upload"
  startedAt?: string
  finishedAt?: string
  emailsScanned: number
  ordersNew: number
  ordersLinked: number
  skipped: number
  warnings: string[]
  error?: string
  resumes?: boolean
}

export interface LedgerCategory {
  id: string
  name: string
  parentId?: string
  matchWords: string[]
  createdAt: string
  updatedAt: string
}

export const ledgerCategoryInputSchema = z.object({
  name: z.string().min(1),
  parentId: z.string().uuid().optional(),
  matchWords: z.array(z.string()).default([]),
})

export type LedgerCategoryInput = z.infer<typeof ledgerCategoryInputSchema>

// A part of a transaction booked to another category; the rest of the amount
// keeps the transaction's own category. categoryId is only missing after the
// category was deleted.
export interface LedgerTransactionSplit {
  amountMinor: number
  categoryId?: string
  note?: string
}

export interface LedgerTransactionReference {
  type: "purchase" | "contract" | "vehicle" | "cost"
  targetId: string
}

export interface LedgerTransaction {
  id: string
  accountId: string
  categoryId?: string
  splits?: LedgerTransactionSplit[]
  specialCategory?: string
  transferPairTransactionId?: string
  bookingDate: string
  valueDate?: string
  amountMinor: number
  currency: string
  counterpartyName?: string
  counterpartyIban?: string
  purpose?: string
  bankReference?: string
  reviewStatus: LedgerReviewStatus
  categorizationSource: LedgerCategorizationSource
  note?: string
  links?: string[]
  references?: LedgerTransactionReference[]
  emailOrderIds?: string[]
  sourceType: LedgerSourceType
  importBatchId: string
  fingerprint: string
  createdAt: string
  updatedAt: string
}

export interface LedgerTransferCandidate {
  transaction: LedgerTransaction
  accountName: string
  dateDeltaDays: number
  ibanMatch: boolean
}

export interface LedgerTransferCandidatesResult {
  items: LedgerTransferCandidate[]
}

export interface LedgerEmailOrderCandidate {
  transaction: LedgerTransaction
  accountName: string
  dateDeltaDays?: number
  amountComparable: boolean
}

export interface LedgerEmailOrderConversion {
  currency: string
  totalMinor: number
  rate: number
  rateDate: string
}

export interface LedgerEmailOrderCandidatesResult {
  items: LedgerEmailOrderCandidate[]
  beforeDays: number
  afterDays: number
  conversions?: LedgerEmailOrderConversion[]
  unconvertedCurrencies?: string[]
}

export interface LedgerTransferLinkInput {
  pairedTransactionId: string
}

// Linking always returns the paired transaction; unlinking only when the
// paired side still exists.
export interface LedgerTransferLinkResult {
  transaction: LedgerTransaction
  pairedTransaction?: LedgerTransaction
}

export interface LedgerTransactionDetailsInput {
  note?: string
  links: string[]
  references: LedgerTransactionReference[]
}

export interface LedgerImportBatch {
  id: string
  accountId: string
  sourceType: LedgerSourceType
  parserVersion: string
  // empty for a bank sync
  filename: string
  fileSha256: string
  status: "parsed" | "committed" | "failed"
  totalRows: number
  importedRows: number
  duplicateRows: number
  errorRows: number
  warnings?: string[]
  createdAt: string
  updatedAt: string
}

export interface LedgerPreviewRow {
  bookingDate: string
  valueDate?: string
  amountMinor: number
  currency: string
  counterpartyName?: string
  counterpartyIban?: string
  purpose?: string
  bankReference?: string
}

export interface LedgerPreviewTransaction {
  row: LedgerPreviewRow
  fingerprint: string
  isDuplicate: boolean
  suggestedCategoryId?: string
  suggestedCategoryName?: string
  reviewStatus: LedgerReviewStatus
  categorizationSource: LedgerCategorizationSource
}

export interface LedgerPreviewResult {
  previewId: string
  sourceType: LedgerFileSourceType
  filename: string
  fileSha256: string
  accountId?: string
  iban?: string
  bankName?: string
  transactions: LedgerPreviewTransaction[]
  totalRows: number
  newRows: number
  duplicateRows: number
  warnings?: string[]
  balance?: { amountMinor: number; asOf: string }
  expiresAt: string
}

export interface LedgerCommitResult {
  batchId: string
  accountId: string
  importedRows: number
  duplicateRows: number
}

// closingMinor is the balance after the page's newest transaction (top),
// openingMinor the balance before its oldest one (bottom).
export interface LedgerPageBalance {
  closingMinor: number
  openingMinor: number
}

// A page of one account's transactions; total counts the account's transactions.
export interface LedgerTransactionsPage {
  items: LedgerTransaction[]
  offset: number
  total: number
  pageBalance?: LedgerPageBalance
  accountBalance?: LedgerAccountBalance
}

// A page of the cross-account list; total counts the matches on all pages.
export interface LedgerTransactionList {
  items: LedgerTransaction[]
  offset: number
  total: number
}

export const ledgerSuggestionConfidences = ["high", "medium", "low"] as const
export type LedgerSuggestionConfidence = (typeof ledgerSuggestionConfidences)[number]

export interface LedgerSuggestion {
  categoryId: string
  confidence: LedgerSuggestionConfidence
  matchWord?: string
}

export interface LedgerReviewQueueItem extends LedgerTransaction {
  suggestion?: LedgerSuggestion
}

export interface LedgerReviewQueuePage {
  items: LedgerReviewQueueItem[]
  nextCursor?: string
  // the size of the whole queue, also for a short page
  total: number
}

export interface LedgerSuggestionRunResult {
  candidates: number
  ruleMatched: number
  suggested: number
  noSuggestion: number
  remaining: number
  partial: boolean
  warnings: string[]
}

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

export interface LedgerReviewResult {
  transaction: LedgerTransaction
  category?: LedgerCategory
}

// categoryId and newCategory are mutually exclusive
export interface LedgerReviewInput {
  categoryId?: string
  newCategory?: LedgerCategoryInput
  addMatchWords: string[]
}

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

// accountId and newAccount are mutually exclusive
export interface LedgerCommitRequest {
  accountId?: string
  newAccount?: LedgerAccountInput
}

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

// Monthly income, expenses and their difference; expensesMinor equals the
// totals of the top-level spending analysis.
export interface LedgerCashFlowAnalysis {
  from: string
  to: string
  months: string[]
  incomeMinor: number[]
  expensesMinor: number[]
  netMinor: number[]
  excludedCurrencies: string[]
}

export interface LedgerRankedCategory {
  kind: "category" | "uncategorized"
  categoryId?: string
  name: string
  parentName?: string
  amountMinor: number
}

export interface LedgerRankedCounterparty {
  name: string
  // transactions without a counterparty name, as one entry
  unknown?: boolean
  amountMinor: number
}

export interface LedgerRankingAnalysis {
  from: string
  to: string
  categories: LedgerRankedCategory[]
  counterparties: LedgerRankedCounterparty[]
  excludedCurrencies: string[]
}

export interface LedgerNetWorthAccount {
  accountId: string
  name: string
  valuesMinor: number[]
}

export interface LedgerNetWorthExcluded {
  accountId: string
  name: string
  reason: "no_balance" | "currency"
  // only for reason currency
  currency?: string
}

// Balances of the EUR accounts that have a balance at each date, and their
// sum; the other accounts are listed in excludedAccounts.
export interface LedgerNetWorthAnalysis {
  from: string
  to: string
  dates: string[]
  totalMinor: number[]
  accounts: LedgerNetWorthAccount[]
  excludedAccounts: LedgerNetWorthExcluded[]
}
