import type { FieldConfig } from "@/modules/contracts/config/contract-fields"

export const purchaseFields: FieldConfig[] = [
  { key: "itemName", type: "text", i18nKey: "purchaseFields.itemName", required: true, showInTable: true, tableOrder: 0 },
  { key: "type", type: "text", i18nKey: "purchaseFields.type", required: false, showInTable: true, tableOrder: 1 },
  { key: "brand", type: "text", i18nKey: "purchaseFields.brand", required: false, showInTable: true, tableOrder: 2 },
  { key: "dealer", type: "text", i18nKey: "purchaseFields.dealer", required: false, showInTable: true, tableOrder: 3 },
  { key: "price", type: "number", i18nKey: "purchaseFields.price", required: false, showInTable: true, tableOrder: 4 },
  { key: "purchaseDate", type: "date", i18nKey: "purchaseFields.purchaseDate", required: false, showInTable: true, tableOrder: 5 },
  { key: "articleNumber", type: "text", i18nKey: "purchaseFields.articleNumber", required: false, showInTable: false, tableOrder: -1 },
  { key: "descriptionUrl", type: "url", i18nKey: "purchaseFields.descriptionUrl", required: false, showInTable: false, tableOrder: -1 },
  { key: "invoiceUrl", type: "url", i18nKey: "purchaseFields.invoiceUrl", required: false, showInTable: false, tableOrder: -1 },
  { key: "handbookUrl", type: "url", i18nKey: "purchaseFields.handbookUrl", required: false, showInTable: false, tableOrder: -1 },
  { key: "consumables", type: "textarea", i18nKey: "purchaseFields.consumables", required: false, showInTable: false, tableOrder: -1 },
  { key: "comments", type: "textarea", i18nKey: "purchaseFields.comments", required: false, showInTable: false, tableOrder: -1 },
]
