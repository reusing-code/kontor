import type { CategoryItemConfig } from "@/components/category-items/types"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { ContractsTable } from "@/modules/contracts/components/contracts-table"
import { contractHooks } from "@/modules/contracts/hooks/use-contracts"
import type { Contract, ContractFormData } from "@/modules/contracts/types"

export const contractItems: CategoryItemConfig<Contract, ContractFormData> = {
  kind: "contracts",
  entityType: "contract",
  itemName: (contract) => contract.name,
  hooks: contractHooks,
  detailFields: [
    { labelKey: "fields.company", value: (c) => c.company },
    { labelKey: "fields.price", value: (c) => c.price, price: true },
    { labelKey: "fields.startDate", value: (c) => c.startDate, date: true },
    { labelKey: "fields.endDate", value: (c) => c.endDate, date: true },
    { labelKey: "fields.comments", value: (c) => c.comments, wide: true },
  ],
  renderTable: ({ items, ...props }) => <ContractsTable contracts={items} {...props} />,
  renderDialog: ({ item, ...props }) => <ContractDialog contract={item} {...props} />,
}
