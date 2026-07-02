import { useTranslation } from "react-i18next"
import type { FieldConfig } from "@/modules/contracts/config/contract-fields"
import type { Control, FieldValues, Path } from "react-hook-form"
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

interface FormFieldRendererProps<T extends FieldValues> {
  config: FieldConfig
  control: Control<T>
}

export function FormFieldRenderer<T extends FieldValues>({ config, control }: FormFieldRendererProps<T>) {
  const { t } = useTranslation()

  return (
    <FormField
      control={control}
      name={config.key as Path<T>}
      render={({ field }) => (
        <FormItem>
          <FormLabel>
            {t(config.i18nKey)}
            {config.required && " *"}
          </FormLabel>
          <FormControl>
            {config.type === "billingInterval" ? (
              <Select
                value={(field.value as string) ?? "monthly"}
                onValueChange={field.onChange}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="monthly">{t("fields.billingMonthly")}</SelectItem>
                  <SelectItem value="yearly">{t("fields.billingYearly")}</SelectItem>
                </SelectContent>
              </Select>
            ) : config.type === "textarea" ? (
              <Textarea
                {...field}
                value={(field.value as string) ?? ""}
                onChange={(e) => field.onChange(e.target.value || undefined)}
              />
            ) : (
              <Input
                type={config.type === "date" ? "date" : "text"}
                inputMode={config.type === "number" ? "decimal" : undefined}
                {...field}
                value={field.value === undefined || field.value === null ? "" : String(field.value)}
                onChange={(e) => field.onChange(e.target.value || undefined)}
              />
            )}
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

// Re-export for backward compatibility
export const ContractFormField = FormFieldRenderer
