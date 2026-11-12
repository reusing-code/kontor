import { useState } from "react"
import { useTranslation } from "react-i18next"
import { addDays, endOfDay, format, parse } from "date-fns"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { tokenScopes, type TokenCreateInput, type TokenScope } from "@/types/token"

type ExpiryChoice = "30" | "90" | "365" | "custom" | "never"

interface TokenCreateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  pending: boolean
  onSubmit: (input: TokenCreateInput) => void
}

function expiryFor(choice: ExpiryChoice, customDate: string): string | undefined {
  switch (choice) {
    case "never":
      return undefined
    case "custom":
      // "yyyy-MM-dd" must be read as a local date: new Date("2026-12-01") is
      // midnight UTC, the previous evening west of Greenwich.
      return endOfDay(parse(customDate, "yyyy-MM-dd", new Date())).toISOString()
    default:
      return addDays(new Date(), Number(choice)).toISOString()
  }
}

export function TokenCreateDialog({ open, onOpenChange, pending, onSubmit }: TokenCreateDialogProps) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("tokens.create")}</DialogTitle>
          <DialogDescription>{t("tokens.createDescription")}</DialogDescription>
        </DialogHeader>
        {/* mounted only while open, so every opening starts with a fresh form */}
        <TokenCreateForm pending={pending} onCancel={() => onOpenChange(false)} onSubmit={onSubmit} />
      </DialogContent>
    </Dialog>
  )
}

function TokenCreateForm({ pending, onCancel, onSubmit }: { pending: boolean; onCancel: () => void; onSubmit: (input: TokenCreateInput) => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState("")
  const [scopes, setScopes] = useState<TokenScope[]>(["read"])
  const [expiry, setExpiry] = useState<ExpiryChoice>("90")
  const [customDate, setCustomDate] = useState("")
  const [submitted, setSubmitted] = useState(false)
  const [minDate] = useState(() => format(addDays(new Date(), 1), "yyyy-MM-dd"))

  const nameProblem = name.trim() === ""
  const scopeProblem = scopes.length === 0
  const expiryProblem = expiry === "custom" && customDate < minDate

  function toggleScope(scope: TokenScope, checked: boolean) {
    setScopes((current) => (checked ? [...current, scope] : current.filter((s) => s !== scope)))
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitted(true)
    if (nameProblem || scopeProblem || expiryProblem) return
    const expiresAt = expiryFor(expiry, customDate)
    onSubmit({
      name: name.trim(),
      scopes: tokenScopes.filter((s) => scopes.includes(s)),
      ...(expiresAt ? { expiresAt } : {}),
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="tokenName">{t("tokens.name")}</Label>
        <Input
          id="tokenName"
          value={name}
          maxLength={100}
          placeholder={t("tokens.namePlaceholder")}
          onChange={(e) => setName(e.target.value)}
          autoComplete="off"
        />
        {submitted && nameProblem && <p className="text-sm text-destructive">{t("tokens.nameRequired")}</p>}
      </div>

      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t("tokens.scopes")}</legend>
        {tokenScopes.map((scope) => (
          <div key={scope} className="flex items-start gap-2">
            <Checkbox
              id={`scope-${scope}`}
              checked={scopes.includes(scope)}
              onCheckedChange={(checked) => toggleScope(scope, checked === true)}
              className="mt-0.5"
            />
            <Label htmlFor={`scope-${scope}`} className="flex flex-col items-start gap-0.5 font-normal">
              <span className="font-medium">{t(`tokens.scope.${scope}`)}</span>
              <span className="text-xs text-muted-foreground">{t(`tokens.scopeHint.${scope}`)}</span>
            </Label>
          </div>
        ))}
        {submitted && scopeProblem && <p className="text-sm text-destructive">{t("tokens.scopeRequired")}</p>}
      </fieldset>

      <div className="space-y-2">
        <Label htmlFor="tokenExpiry">{t("tokens.expiry")}</Label>
        <Select value={expiry} onValueChange={(v) => setExpiry(v as ExpiryChoice)}>
          <SelectTrigger id="tokenExpiry" className="w-60">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="30">{t("tokens.expiryDays", { count: 30 })}</SelectItem>
            <SelectItem value="90">{t("tokens.expiryDays", { count: 90 })}</SelectItem>
            <SelectItem value="365">{t("tokens.expiryDays", { count: 365 })}</SelectItem>
            <SelectItem value="custom">{t("tokens.expiryCustom")}</SelectItem>
            <SelectItem value="never">{t("tokens.expiryNever")}</SelectItem>
          </SelectContent>
        </Select>
        {expiry === "custom" && (
          <Input
            type="date"
            aria-label={t("tokens.expiryCustom")}
            min={minDate}
            value={customDate}
            onChange={(e) => setCustomDate(e.target.value)}
            className="w-60"
          />
        )}
        {submitted && expiryProblem && <p className="text-sm text-destructive">{t("tokens.expiryInvalid")}</p>}
        {expiry === "never" && (
          <p className="text-sm text-amber-600 dark:text-amber-400" role="alert">
            {t("tokens.neverWarning")}
          </p>
        )}
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={pending}>
          {t("tokens.createButton")}
        </Button>
      </DialogFooter>
</form>
  )
}
