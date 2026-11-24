import { formatDate, formatMoney } from "@/lib/format"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Link2 } from "lucide-react"
import { useContracts } from "@/modules/contracts/hooks/use-contracts"
import { usePurchases } from "@/modules/purchases/hooks/use-purchases"
import { useCostEntry, useVehicles } from "@/modules/auto/hooks/use-vehicles"
import { moduleReferenceToPath, REFERENCE_NAV_KEY, vehiclePath } from "@/modules/ledger/lib/module-links"
import type { LedgerTransactionReference } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"

function ReferenceRowShell({ reference, label, description, hint, path = moduleReferenceToPath(reference), onRemove }: {
  reference: LedgerTransactionReference
  label: string
  description?: string
  hint?: string
  path?: string | null
  onRemove: () => void
}) {
  const { t } = useTranslation()

  return (
    <div className="flex items-center gap-2 rounded-md border p-3 text-sm">
      <Link2 className="h-4 w-4 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        {hint === undefined && path !== null ? (
          <Link to={path} className="block truncate text-primary hover:underline">
            {label}
          </Link>
        ) : (
          <div className="block truncate">
            {label} {hint ? <span className="text-muted-foreground">{hint}</span> : null}
          </div>
        )}
        {description ? <div className="truncate text-xs text-muted-foreground">{description}</div> : null}
      </div>
      <Button type="button" variant="ghost" size="sm" onClick={onRemove}>
        {t("common.delete")}
      </Button>
    </div>
  )
}

function PurchaseReferenceRow({ reference, onRemove }: { reference: LedgerTransactionReference; onRemove: () => void }) {
  const { t } = useTranslation()
  const { data: purchases = [] } = usePurchases()
  const purchase = purchases.find((item) => item.id === reference.targetId)
  const label = purchase?.itemName ?? `${t("nav.purchases")} ${reference.targetId}`
  const description = purchase ? [purchase.dealer, purchase.purchaseDate].filter(Boolean).join(" • ") : undefined
  return <ReferenceRowShell reference={reference} label={label} description={description} onRemove={onRemove} />
}

function ContractReferenceRow({ reference, onRemove }: { reference: LedgerTransactionReference; onRemove: () => void }) {
  const { t } = useTranslation()
  const { data: contracts = [] } = useContracts()
  const contract = contracts.find((item) => item.id === reference.targetId)
  const label = contract?.name ?? `${t("nav.contracts")} ${reference.targetId}`
  const description = contract ? [contract.company, contract.startDate].filter(Boolean).join(" • ") : undefined
  return <ReferenceRowShell reference={reference} label={label} description={description} onRemove={onRemove} />
}

function VehicleReferenceRow({ reference, onRemove }: { reference: LedgerTransactionReference; onRemove: () => void }) {
  const { t } = useTranslation()
  const { data: vehicles = [] } = useVehicles()
  const vehicle = vehicles.find((item) => item.id === reference.targetId)
  const label = vehicle?.name ?? `${t("nav.auto")} ${reference.targetId}`
  const description = vehicle ? [vehicle.make, vehicle.model, vehicle.licensePlate].filter(Boolean).join(" • ") : undefined
  return <ReferenceRowShell reference={reference} label={label} description={description} onRemove={onRemove} />
}

function CostReferenceRow({ reference, onRemove }: { reference: LedgerTransactionReference; onRemove: () => void }) {
  const { t } = useTranslation()
  const { data: cost } = useCostEntry(reference.targetId)
  const { data: vehicles = [] } = useVehicles()
  const vehicle = cost ? vehicles.find((item) => item.id === cost.vehicleId) : undefined
  const label = cost ? cost.description || t(`costTypes.${cost.type}`) : `${t("ledger.referenceTypes.cost")} ${reference.targetId}`
  const description = cost
    ? [
        vehicle?.name,
        formatDate(cost.date),
        cost.amount != null ? formatMoney(cost.amount) : undefined,
      ].filter(Boolean).join(" • ")
    : undefined
  return (
    <ReferenceRowShell
      reference={reference}
      label={label}
      description={description}
      path={cost ? vehiclePath(cost.vehicleId) : null}
      onRemove={onRemove}
    />
  )
}

export function ReferenceRow({ reference, enabled, onRemove }: {
  reference: LedgerTransactionReference
  enabled: boolean
  onRemove: () => void
}) {
  const { t } = useTranslation()

  if (!enabled) {
    const label = `${t(REFERENCE_NAV_KEY[reference.type])} ${reference.targetId}`
    return <ReferenceRowShell reference={reference} label={label} hint={t("modules.disabledHint")} onRemove={onRemove} />
  }

  switch (reference.type) {
    case "purchase":
      return <PurchaseReferenceRow reference={reference} onRemove={onRemove} />
    case "contract":
      return <ContractReferenceRow reference={reference} onRemove={onRemove} />
    case "vehicle":
      return <VehicleReferenceRow reference={reference} onRemove={onRemove} />
    case "cost":
      return <CostReferenceRow reference={reference} onRemove={onRemove} />
  }
}
