export type FieldType = "text" | "number" | "date" | "url" | "textarea" | "billingInterval"

export interface FieldConfig {
  key: string
  type: FieldType
  i18nKey: string
  required: boolean
  showInTable: boolean
  tableOrder: number
}

export const contractFields: FieldConfig[] = [
  { key: "name", type: "text", i18nKey: "fields.name", required: true, showInTable: true, tableOrder: 0 },
  { key: "productName", type: "text", i18nKey: "fields.productName", required: false, showInTable: true, tableOrder: 1 },
  { key: "company", type: "text", i18nKey: "fields.company", required: false, showInTable: true, tableOrder: 2 },
  { key: "contractNumber", type: "text", i18nKey: "fields.contractNumber", required: false, showInTable: false, tableOrder: -1 },
  { key: "customerNumber", type: "text", i18nKey: "fields.customerNumber", required: false, showInTable: false, tableOrder: -1 },
  { key: "price", type: "number", i18nKey: "fields.price", required: false, showInTable: true, tableOrder: 3 },
  { key: "billingInterval", type: "billingInterval", i18nKey: "fields.billingInterval", required: true, showInTable: false, tableOrder: -1 },
  { key: "startDate", type: "date", i18nKey: "fields.startDate", required: true, showInTable: true, tableOrder: 4 },
  { key: "endDate", type: "date", i18nKey: "fields.endDate", required: false, showInTable: false, tableOrder: -1 },
  { key: "minimumDurationMonths", type: "number", i18nKey: "fields.minimumDuration", required: true, showInTable: false, tableOrder: -1 },
  { key: "extensionDurationMonths", type: "number", i18nKey: "fields.extensionDuration", required: true, showInTable: false, tableOrder: -1 },
  { key: "noticePeriodMonths", type: "number", i18nKey: "fields.noticePeriod", required: true, showInTable: false, tableOrder: -1 },
  { key: "customerPortalUrl", type: "url", i18nKey: "fields.customerPortalUrl", required: false, showInTable: false, tableOrder: -1 },
  { key: "paperlessUrl", type: "url", i18nKey: "fields.paperlessUrl", required: false, showInTable: false, tableOrder: -1 },
  { key: "comments", type: "textarea", i18nKey: "fields.comments", required: false, showInTable: false, tableOrder: -1 },
]
