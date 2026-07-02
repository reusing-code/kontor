import type { FieldConfig } from "@/modules/contracts/config/contract-fields"

export const vehicleFields: FieldConfig[] = [
  { key: "name", type: "text", i18nKey: "vehicleFields.name", required: true, showInTable: true, tableOrder: 0 },
  { key: "make", type: "text", i18nKey: "vehicleFields.make", required: false, showInTable: true, tableOrder: 1 },
  { key: "model", type: "text", i18nKey: "vehicleFields.model", required: false, showInTable: true, tableOrder: 2 },
  { key: "year", type: "number", i18nKey: "vehicleFields.year", required: false, showInTable: true, tableOrder: 3 },
  { key: "licensePlate", type: "text", i18nKey: "vehicleFields.licensePlate", required: false, showInTable: true, tableOrder: 4 },
  { key: "purchaseDate", type: "date", i18nKey: "vehicleFields.purchaseDate", required: false, showInTable: false, tableOrder: -1 },
  { key: "purchasePrice", type: "number", i18nKey: "vehicleFields.purchasePrice", required: false, showInTable: false, tableOrder: -1 },
  { key: "purchaseMileage", type: "number", i18nKey: "vehicleFields.purchaseMileage", required: false, showInTable: false, tableOrder: -1 },
  { key: "targetMileage", type: "number", i18nKey: "vehicleFields.targetMileage", required: false, showInTable: false, tableOrder: -1 },
  { key: "targetMonths", type: "number", i18nKey: "vehicleFields.targetMonths", required: false, showInTable: false, tableOrder: -1 },
  { key: "annualInsurance", type: "number", i18nKey: "vehicleFields.annualInsurance", required: false, showInTable: false, tableOrder: -1 },
  { key: "annualTax", type: "number", i18nKey: "vehicleFields.annualTax", required: false, showInTable: false, tableOrder: -1 },
  { key: "maintenanceFactor", type: "number", i18nKey: "vehicleFields.maintenanceFactor", required: false, showInTable: false, tableOrder: -1 },
  { key: "comments", type: "textarea", i18nKey: "vehicleFields.comments", required: false, showInTable: false, tableOrder: -1 },
]
