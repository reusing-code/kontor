import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { useForm, type Resolver } from "react-hook-form"
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
import { LedgerCategorySelect } from "@/modules/ledger/components/ledger-category-select"
import { descendantIds } from "@/modules/ledger/lib/category-tree"

interface LedgerCategoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  category?: LedgerCategory | null
  categories: LedgerCategory[]
  initialParentId?: string
  // Prefills the create form (name, match words); ignored when editing.
  initialValues?: Partial<Pick<LedgerCategoryInput, "name" | "matchWords">>
  // Save stays disabled until a returned promise settles; closing is left to
  // the caller, a rejection to the global mutation error toast.
  onSubmit: (data: LedgerCategoryInput) => void | Promise<unknown>
}

export function LedgerCategoryDialog({
  open,
  onOpenChange,
  category,
  categories,
  initialParentId,
  initialValues,
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
          initialValues={initialValues}
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
  initialValues?: Partial<Pick<LedgerCategoryInput, "name" | "matchWords">>
  onOpenChange: (open: boolean) => void
  onSubmit: LedgerCategoryDialogProps["onSubmit"]
}

function LedgerCategoryDialogForm({
  category,
  categories,
  initialParentId,
  initialValues,
  onOpenChange,
  onSubmit,
}: LedgerCategoryDialogFormProps) {
  const { t } = useTranslation()
  // a category cannot move below itself
  const ownSubtree = useMemo(() => (category ? descendantIds(categories, category.id) : undefined), [categories, category])
  const [matchWordsInput, setMatchWordsInput] = useState((category?.matchWords ?? initialValues?.matchWords ?? []).join(", "))
  const form = useForm<LedgerCategoryInput>({
    resolver: standardSchemaResolver(ledgerCategoryInputSchema) as Resolver<LedgerCategoryInput>,
    defaultValues: {
      name: category?.name ?? initialValues?.name ?? "",
      parentId: category?.parentId ?? initialParentId,
      matchWords: category?.matchWords ?? initialValues?.matchWords ?? [],
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
          onSubmit={form.handleSubmit(async (data) => {
            const matchWords = matchWordsInput
              .split(",")
              .map((word) => word.trim())
              .filter(Boolean)
            try {
              await onSubmit({ name: data.name, parentId: data.parentId, matchWords })
            } catch {
              // the global mutation error toast reports it
            }
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
                <FormControl>
                  <LedgerCategorySelect
                    categories={categories}
                    value={field.value ?? ""}
                    onValueChange={(value) => field.onChange(value || undefined)}
                    noneLabel={t("ledger.noParentCategory")}
                    exclude={ownSubtree}
                  />
                </FormControl>
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
            <Button type="submit" disabled={form.formState.isSubmitting}>{t("common.save")}</Button>
          </DialogFooter>
        </form>
      </Form>
    </DialogContent>
  )
}
