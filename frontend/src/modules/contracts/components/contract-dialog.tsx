import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import { useForm, type Resolver } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { contractFormSchema, type ContractFormData, type Contract } from "@/modules/contracts/types"
import { contractFields } from "@/modules/contracts/config/contract-fields"
import { FormFieldRenderer } from "@/modules/contracts/components/contract-form-field"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Form } from "@/components/ui/form"
import { Button } from "@/components/ui/button"

interface ContractDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  contract?: Contract | null
  onSubmit: (data: ContractFormData) => void
}

const defaultValues: ContractFormData = {
  name: "",
  startDate: new Date().toISOString().slice(0, 10),
  billingInterval: "monthly",
  minimumDurationMonths: 12,
  extensionDurationMonths: 12,
  noticePeriodMonths: 3,
}

export function ContractDialog({ open, onOpenChange, contract, onSubmit }: ContractDialogProps) {
  const { t } = useTranslation()
  const form = useForm<ContractFormData>({
    resolver: standardSchemaResolver(contractFormSchema) as Resolver<ContractFormData>,
    defaultValues,
  })

  useEffect(() => {
    if (open) {
      if (contract) {
        form.reset({
          name: contract.name,
          productName: contract.productName,
          company: contract.company,
          contractNumber: contract.contractNumber,
          customerNumber: contract.customerNumber,
          price: contract.price,
          billingInterval: contract.billingInterval,
          startDate: contract.startDate,
          endDate: contract.endDate,
          minimumDurationMonths: contract.minimumDurationMonths,
          extensionDurationMonths: contract.extensionDurationMonths,
          noticePeriodMonths: contract.noticePeriodMonths,
          customerPortalUrl: contract.customerPortalUrl,
          paperlessUrl: contract.paperlessUrl,
          comments: contract.comments,
        })
      } else {
        form.reset(defaultValues)
      }
    }
  }, [open, contract, form])

  function handleSubmit(data: ContractFormData) {
    onSubmit(data)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {contract ? t("contract.edit") : t("contract.create")}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            {contractFields.map((field) => (
              <FormFieldRenderer<ContractFormData> key={field.key} config={field} control={form.control} />
            ))}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit">{t("common.save")}</Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
