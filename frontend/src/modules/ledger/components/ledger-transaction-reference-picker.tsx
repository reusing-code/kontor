import { useState } from "react"
import { useTranslation } from "react-i18next"
import { useContracts } from "@/modules/contracts/hooks/use-contracts"
import { usePurchases } from "@/modules/purchases/hooks/use-purchases"
import { useVehicles } from "@/modules/auto/hooks/use-vehicles"
import { REFERENCE_NAV_KEY, type ReferenceType } from "@/modules/ledger/lib/module-links"
import type { LedgerTransactionReference } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

type TargetOption = { id: string; label: string }

function TargetSelect({ options, value, onChange, placeholder }: {
  options: TargetOption[]
  value: string
  onChange: (value: string) => void
  placeholder: string
}) {
  return (
    <Select value={value || undefined} onValueChange={onChange}>
      <SelectTrigger className="w-full">
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

type TargetSelectProps = {
  value: string
  onChange: (value: string) => void
  placeholder: string
}

function PurchaseTargetSelect(props: TargetSelectProps) {
  const { data: purchases = [] } = usePurchases()
  return <TargetSelect options={purchases.map((purchase) => ({ id: purchase.id, label: purchase.itemName }))} {...props} />
}

function ContractTargetSelect(props: TargetSelectProps) {
  const { data: contracts = [] } = useContracts()
  return <TargetSelect options={contracts.map((contract) => ({ id: contract.id, label: contract.name }))} {...props} />
}

function VehicleTargetSelect(props: TargetSelectProps) {
  const { data: vehicles = [] } = useVehicles()
  return <TargetSelect options={vehicles.map((vehicle) => ({ id: vehicle.id, label: vehicle.name }))} {...props} />
}

export function LedgerTransactionReferencePicker({ availableTypes, onAdd }: {
  availableTypes: ReferenceType[]
  onAdd: (reference: LedgerTransactionReference) => void
}) {
  const { t } = useTranslation()
  const [referenceType, setReferenceType] = useState<ReferenceType>("purchase")
  const [referenceTargetId, setReferenceTargetId] = useState("")

  const effectiveReferenceType = availableTypes.includes(referenceType) ? referenceType : availableTypes[0]

  function handleAddReference() {
    if (!effectiveReferenceType || !referenceTargetId) {
      return
    }
    onAdd({ type: effectiveReferenceType, targetId: referenceTargetId })
    setReferenceTargetId("")
  }

  if (effectiveReferenceType === undefined) {
    return null
  }

  return (
    <div className="grid gap-2 md:grid-cols-[12rem_1fr_auto]">
      <Select
        value={effectiveReferenceType}
        onValueChange={(value) => { setReferenceType(value as ReferenceType); setReferenceTargetId("") }}
      >
        <SelectTrigger className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {availableTypes.map((type) => (
            <SelectItem key={type} value={type}>{t(REFERENCE_NAV_KEY[type])}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      {effectiveReferenceType === "purchase" && (
        <PurchaseTargetSelect value={referenceTargetId} onChange={setReferenceTargetId} placeholder={t("ledger.selectReferenceTarget")} />
      )}
      {effectiveReferenceType === "contract" && (
        <ContractTargetSelect value={referenceTargetId} onChange={setReferenceTargetId} placeholder={t("ledger.selectReferenceTarget")} />
      )}
      {effectiveReferenceType === "vehicle" && (
        <VehicleTargetSelect value={referenceTargetId} onChange={setReferenceTargetId} placeholder={t("ledger.selectReferenceTarget")} />
      )}
      <Button type="button" variant="outline" onClick={handleAddReference}>{t("ledger.addReference")}</Button>
    </div>
  )
}
