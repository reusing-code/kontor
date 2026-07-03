import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import { useForm } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { vehicleFormSchema, type VehicleFormData, type Vehicle } from "@/modules/auto/types"
import { vehicleFields } from "@/modules/auto/config/vehicle-fields"
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

interface VehicleDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  vehicle?: Vehicle | null
  onSubmit: (data: VehicleFormData) => void
}

const defaultValues: VehicleFormData = {
  name: "",
  purchaseDate: new Date().toISOString().slice(0, 10),
}

export function VehicleDialog({ open, onOpenChange, vehicle, onSubmit }: VehicleDialogProps) {
  const { t } = useTranslation()
  const form = useForm<VehicleFormData>({
    resolver: standardSchemaResolver(vehicleFormSchema),
    defaultValues,
  })

  useEffect(() => {
    if (open) {
      if (vehicle) {
        form.reset({
          name: vehicle.name,
          make: vehicle.make,
          model: vehicle.model,
          year: vehicle.year,
          licensePlate: vehicle.licensePlate,
          purchaseDate: vehicle.purchaseDate,
          purchasePrice: vehicle.purchasePrice,
          purchaseMileage: vehicle.purchaseMileage,
          targetMileage: vehicle.targetMileage,
          targetMonths: vehicle.targetMonths,
          annualInsurance: vehicle.annualInsurance,
          annualTax: vehicle.annualTax,
          maintenanceFactor: vehicle.maintenanceFactor,
          comments: vehicle.comments,
        })
      } else {
        form.reset(defaultValues)
      }
    }
  }, [open, vehicle, form])

  function handleSubmit(data: VehicleFormData) {
    onSubmit(data)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {vehicle ? t("vehicle.edit") : t("vehicle.create")}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            {vehicleFields.map((field) => (
              <FormFieldRenderer<VehicleFormData> key={field.key} config={field} control={form.control} />
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
