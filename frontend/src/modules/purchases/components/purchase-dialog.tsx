import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import { useForm, type Resolver } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { purchaseFormSchema, type PurchaseFormData, type Purchase } from "@/modules/purchases/types"
import { purchaseFields } from "@/modules/purchases/config/purchase-fields"
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

interface PurchaseDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  purchase?: Purchase | null
  onSubmit: (data: PurchaseFormData) => void
}

const defaultValues: PurchaseFormData = {
  itemName: "",
  purchaseDate: new Date().toISOString().slice(0, 10),
}

export function PurchaseDialog({ open, onOpenChange, purchase, onSubmit }: PurchaseDialogProps) {
  const { t } = useTranslation()
  const form = useForm<PurchaseFormData>({
    resolver: standardSchemaResolver(purchaseFormSchema) as Resolver<PurchaseFormData>,
    defaultValues,
  })

  useEffect(() => {
    if (open) {
      if (purchase) {
        form.reset({
          type: purchase.type,
          itemName: purchase.itemName,
          brand: purchase.brand,
          articleNumber: purchase.articleNumber,
          dealer: purchase.dealer,
          price: purchase.price,
          purchaseDate: purchase.purchaseDate,
          descriptionUrl: purchase.descriptionUrl,
          invoiceUrl: purchase.invoiceUrl,
          handbookUrl: purchase.handbookUrl,
          consumables: purchase.consumables,
          comments: purchase.comments,
        })
      } else {
        form.reset(defaultValues)
      }
    }
  }, [open, purchase, form])

  function handleSubmit(data: PurchaseFormData) {
    onSubmit(data)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {purchase ? t("purchase.edit") : t("purchase.create")}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            {purchaseFields.map((field) => (
              <FormFieldRenderer<PurchaseFormData> key={field.key} config={field} control={form.control} />
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
