import type { FieldConfig } from "@/modules/contracts/config/contract-fields"

export const costEntryFields: FieldConfig[] = [
  { key: "date", type: "date", i18nKey: "costEntryFields.date", required: true, showInTable: true, tableOrder: 0 },
  { key: "type", type: "text", i18nKey: "costEntryFields.type", required: true, showInTable: true, tableOrder: 1 },
  { key: "description", type: "text", i18nKey: "costEntryFields.description", required: false, showInTable: true, tableOrder: 2 },
  { key: "vendor", type: "text", i18nKey: "costEntryFields.vendor", required: false, showInTable: true, tableOrder: 3 },
  { key: "amount", type: "number", i18nKey: "costEntryFields.amount", required: false, showInTable: true, tableOrder: 4 },
  { key: "mileage", type: "number", i18nKey: "costEntryFields.mileage", required: false, showInTable: true, tableOrder: 5 },
  { key: "comments", type: "textarea", i18nKey: "costEntryFields.comments", required: false, showInTable: false, tableOrder: -1 },
]
