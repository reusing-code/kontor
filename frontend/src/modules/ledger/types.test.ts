import { describe, expect, it } from "vitest"
import {
  ledgerAccountInputSchema,
  ledgerAccountUpdateInputSchema,
  ledgerCategoryInputSchema,
  ledgerEmailAccountInputSchema,
} from "@/modules/ledger/types"
import { issuePaths, issues } from "@/test/schema"

describe("ledgerCategoryInputSchema", () => {
  it("takes a name and defaults the match words", () => {
    expect(ledgerCategoryInputSchema.parse({ name: "Food" })).toEqual({ name: "Food", matchWords: [] })
    expect(issuePaths(ledgerCategoryInputSchema, { name: "Groceries", parentId: "11111111-1111-4111-8111-111111111111" })).toEqual([])
  })

  it("refuses an empty name and a parent that is no ID", () => {
    expect(issuePaths(ledgerCategoryInputSchema, { name: "" })).toEqual(["name"])
    expect(issuePaths(ledgerCategoryInputSchema, { name: "Food", parentId: "food" })).toEqual(["parentId"])
    expect(issuePaths(ledgerCategoryInputSchema, { name: "Food", matchWords: "aldi" })).toEqual(["matchWords"])
  })
})

describe("ledgerAccountInputSchema", () => {
  it("takes name and bank and defaults the currency", () => {
    expect(ledgerAccountInputSchema.parse({ name: "Giro", bank: "DKB" })).toEqual({ name: "Giro", bank: "DKB", currency: "EUR" })
  })

  it.each([
    ["name", { name: "" }],
    ["bank", { bank: "" }],
    ["currency", { currency: "" }],
  ])("refuses an empty %s", (path, change) => {
    expect(issuePaths(ledgerAccountInputSchema, { name: "Giro", bank: "DKB", ...change })).toEqual([path])
  })
})

describe("ledgerAccountUpdateInputSchema", () => {
  it("trims name and bank and refuses blank or overlong ones", () => {
    expect(ledgerAccountUpdateInputSchema.parse({ name: " Giro ", bank: " DKB " })).toEqual({ name: "Giro", bank: "DKB" })
    expect(issuePaths(ledgerAccountUpdateInputSchema, { name: "   ", bank: "DKB" })).toEqual(["name"])
    expect(issuePaths(ledgerAccountUpdateInputSchema, { name: "Giro", bank: "x".repeat(201) })).toEqual(["bank"])
    expect(issuePaths(ledgerAccountUpdateInputSchema, { name: "x".repeat(200), bank: "DKB" })).toEqual([])
  })
})

describe("ledgerEmailAccountInputSchema", () => {
  const minimal = { name: "Mail", imapHost: "imap.example", username: "me", folder: "Orders", scanSince: "2026-01-01" }

  it("takes a minimal account with the defaults", () => {
    expect(ledgerEmailAccountInputSchema.parse(minimal)).toEqual({
      ...minimal,
      imapPort: 993,
      useTls: true,
      draftsFolder: "",
      processedAction: "keep",
      processedFolder: "",
      draftsProcessedAction: "keep",
      draftsProcessedFolder: "",
      preClassify: false,
    })
  })

  it.each([
    ["name", { name: "" }],
    ["imapHost", { imapHost: "" }],
    ["imapPort", { imapPort: 0 }],
    ["imapPort", { imapPort: 99.5 }],
    ["username", { username: "" }],
    ["folder", { folder: "   " }],
    ["scanSince", { scanSince: "" }],
    ["processedAction", { processedAction: "archive" }],
  ])("refuses an invalid %s", (path, change) => {
    expect(issuePaths(ledgerEmailAccountInputSchema, { ...minimal, ...change })).toEqual([path])
  })

  it("refuses the order folder as the drafts folder, also with spaces around it", () => {
    expect(issues(ledgerEmailAccountInputSchema, { ...minimal, draftsFolder: " Orders " })).toEqual([
      { path: "draftsFolder", message: "ledger.email.draftsFolderSame" },
    ])
    expect(issuePaths(ledgerEmailAccountInputSchema, { ...minimal, draftsFolder: "Kontor" })).toEqual([])
  })

  it("needs a move target for the order folder that is neither scanned folder", () => {
    const move = { ...minimal, draftsFolder: "Kontor", processedAction: "move" }
    expect(issues(ledgerEmailAccountInputSchema, move)).toEqual([
      { path: "processedFolder", message: "ledger.email.processed.targetRequired" },
    ])
    for (const processedFolder of ["Orders", "Kontor", " Kontor "]) {
      expect(issues(ledgerEmailAccountInputSchema, { ...move, processedFolder })).toEqual([
        { path: "processedFolder", message: "ledger.email.processed.targetSame" },
      ])
    }
    expect(issuePaths(ledgerEmailAccountInputSchema, { ...move, processedFolder: "Archive" })).toEqual([])
  })

  it("needs a move target for the drafts folder that is neither scanned folder", () => {
    const move = { ...minimal, draftsFolder: "Kontor", draftsProcessedAction: "move" }
    expect(issues(ledgerEmailAccountInputSchema, move)).toEqual([
      { path: "draftsProcessedFolder", message: "ledger.email.processed.targetRequired" },
    ])
    expect(issues(ledgerEmailAccountInputSchema, { ...move, draftsProcessedFolder: "Orders" })).toEqual([
      { path: "draftsProcessedFolder", message: "ledger.email.processed.targetSame" },
    ])
    expect(issuePaths(ledgerEmailAccountInputSchema, { ...move, draftsProcessedFolder: "Done" })).toEqual([])
  })

  it("ignores the move target while the action is not move", () => {
    for (const processedAction of ["keep", "delete"]) {
      expect(issuePaths(ledgerEmailAccountInputSchema, { ...minimal, processedAction, processedFolder: "Orders" })).toEqual([])
    }
  })

  it("ignores the drafts action without a drafts folder", () => {
    expect(issuePaths(ledgerEmailAccountInputSchema, { ...minimal, draftsProcessedAction: "move" })).toEqual([])
  })

  it("reports both move targets at once", () => {
    expect(issuePaths(ledgerEmailAccountInputSchema, {
      ...minimal, draftsFolder: "Kontor", processedAction: "move", draftsProcessedAction: "move", draftsProcessedFolder: "Kontor",
    })).toEqual(["processedFolder", "draftsProcessedFolder"])
  })
})
