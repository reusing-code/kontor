import type { CategoryItemConfig } from "@/components/category-items/types"
import { PurchaseDialog } from "@/modules/purchases/components/purchase-dialog"
import { PurchasesTable } from "@/modules/purchases/components/purchases-table"
import { purchaseHooks } from "@/modules/purchases/hooks/use-purchases"
import type { Purchase, PurchaseFormData } from "@/modules/purchases/types"

export const purchaseItems: CategoryItemConfig<Purchase, PurchaseFormData> = {
  kind: "purchases",
  entityType: "purchase",
  itemName: (purchase) => purchase.itemName,
  hooks: purchaseHooks,
  detailFields: [
    { labelKey: "purchaseFields.type", value: (p) => p.type },
    { labelKey: "purchaseFields.dealer", value: (p) => p.dealer },
    { labelKey: "purchaseFields.price", value: (p) => p.price, price: true },
    { labelKey: "purchaseFields.purchaseDate", value: (p) => p.purchaseDate, date: true },
    { labelKey: "purchaseFields.comments", value: (p) => p.comments, wide: true },
  ],
  renderTable: ({ items, ...props }) => <PurchasesTable purchases={items} {...props} />,
  renderDialog: ({ item, ...props }) => <PurchaseDialog purchase={item} {...props} />,
}
