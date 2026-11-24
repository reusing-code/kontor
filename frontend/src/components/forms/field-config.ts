export type FieldType = "text" | "number" | "date" | "url" | "textarea" | "billingInterval"

export interface FieldConfig {
  key: string
  type: FieldType
  i18nKey: string
  required: boolean
  showInTable: boolean
  tableOrder: number
}
