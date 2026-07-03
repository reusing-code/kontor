import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import { useForm, type Resolver } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { costEntryFormSchema, type CostEntryFormData, type CostEntry, type CostType } from "@/modules/auto/types"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Button } from "@/components/ui/button"

interface CostEntryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  costEntry?: CostEntry | null
  onSubmit: (data: CostEntryFormData) => void
}

const costTypes: CostType[] = [
  "service",
  "fuel",
  "insurance",
  "tax",
  "tires",
  "mileage",
  "misc",
]

const defaultValues: CostEntryFormData = {
  type: "service",
  date: new Date().toISOString().slice(0, 10),
}

export function CostEntryDialog({ open, onOpenChange, costEntry, onSubmit }: CostEntryDialogProps) {
  const { t } = useTranslation()
  const form = useForm<CostEntryFormData>({
    resolver: standardSchemaResolver(costEntryFormSchema) as Resolver<CostEntryFormData>,
    defaultValues,
  })

  useEffect(() => {
    if (open) {
      if (costEntry) {
        form.reset({
          type: costEntry.type,
          description: costEntry.description,
          vendor: costEntry.vendor,
          amount: costEntry.amount,
          date: costEntry.date,
          mileage: costEntry.mileage,
          comments: costEntry.comments,
        })
      } else {
        form.reset(defaultValues)
      }
    }
  }, [open, costEntry, form])

  function handleSubmit(data: CostEntryFormData) {
    onSubmit(data)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {costEntry ? t("costEntry.edit") : t("costEntry.create")}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="type"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("costEntryFields.type")} *</FormLabel>
                  <FormControl>
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {costTypes.map((type) => (
                          <SelectItem key={type} value={type}>
                            {t(`costTypes.${type}`)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="date"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("costEntryFields.date")} *</FormLabel>
                  <FormControl>
                    <Input type="date" {...field} value={field.value ?? ""} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("costEntryFields.description")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ""}
                      onChange={(e) => field.onChange(e.target.value || undefined)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="vendor"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("costEntryFields.vendor")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ""}
                      onChange={(e) => field.onChange(e.target.value || undefined)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="amount"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("costEntryFields.amount")}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      value={field.value === undefined || field.value === null ? "" : String(field.value)}
                      onChange={(e) => field.onChange(e.target.value === "" ? undefined : Number(e.target.value))}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="mileage"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("costEntryFields.mileage")}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      value={field.value === undefined || field.value === null ? "" : String(field.value)}
                      onChange={(e) => field.onChange(e.target.value === "" ? undefined : Number(e.target.value))}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="comments"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("costEntryFields.comments")}</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      value={field.value ?? ""}
                      onChange={(e) => field.onChange(e.target.value || undefined)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
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
