import { useState } from "react"
import { useTranslation } from "react-i18next"
import { useForm } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import type { LedgerCategory, LedgerCategoryInput } from "@/modules/ledger/types"
import { ledgerCategoryInputSchema } from "@/modules/ledger/types"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

interface LedgerCategoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  category?: LedgerCategory | null
  categories: LedgerCategory[]
  initialParentId?: string
  onSubmit: (data: LedgerCategoryInput) => void
}

export function LedgerCategoryDialog({
  open,
  onOpenChange,
  category,
  categories,
  initialParentId,
  onSubmit,
}: LedgerCategoryDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <LedgerCategoryDialogForm
          key={category?.id ?? "new"}
          category={category}
          categories={categories}
          initialParentId={initialParentId}
          onOpenChange={onOpenChange}
          onSubmit={onSubmit}
        />
      ) : null}
    </Dialog>
  )
}

interface LedgerCategoryDialogFormProps {
  category?: LedgerCategory | null
  categories: LedgerCategory[]
  initialParentId?: string
  onOpenChange: (open: boolean) => void
  onSubmit: (data: LedgerCategoryInput) => void
}

function LedgerCategoryDialogForm({
  category,
  categories,
  initialParentId,
  onOpenChange,
  onSubmit,
}: LedgerCategoryDialogFormProps) {
  const { t } = useTranslation()
  const [matchWordsInput, setMatchWordsInput] = useState((category?.matchWords ?? []).join(", "))
  const form = useForm<LedgerCategoryInput>({
    resolver: standardSchemaResolver(ledgerCategoryInputSchema),
    defaultValues: {
      name: category?.name ?? "",
      parentId: category?.parentId ?? initialParentId,
      matchWords: category?.matchWords ?? [],
    },
  })

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{category ? t("ledger.editCategory") : t("ledger.createCategory")}</DialogTitle>
      </DialogHeader>
      <Form {...form}>
        <form
          className="space-y-4"
          onSubmit={form.handleSubmit((data) => {
            const matchWords = matchWordsInput
              .split(",")
              .map((word) => word.trim())
              .filter(Boolean)
            onSubmit({
              name: data.name,
              parentId: data.parentId,
              matchWords,
            })
          })}
        >
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("category.name")}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder={t("ledger.categoryNamePlaceholder")} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="parentId"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.parentCategory")}</FormLabel>
                <Select value={field.value ?? "none"} onValueChange={(value) => field.onChange(value === "none" ? undefined : value)}>
                  <FormControl>
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder={t("ledger.noParentCategory")} />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="none">{t("ledger.noParentCategory")}</SelectItem>
                    {categories
                      .filter((item) => item.id !== category?.id)
                      .map((item) => (
                        <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>
                      ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="matchWords"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.matchWords")}</FormLabel>
                <FormControl>
                  <Input
                    value={matchWordsInput}
                    onChange={(event) => {
                      setMatchWordsInput(event.target.value)
                      field.onChange(event.target.value.split(","))
                    }}
                    placeholder={t("ledger.matchWordsPlaceholder")}
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
  )
}
