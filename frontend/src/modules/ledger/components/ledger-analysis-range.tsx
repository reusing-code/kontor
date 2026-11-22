import { useTranslation } from "react-i18next"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { analysisPresets, resolveRange, type AnalysisRangeState } from "@/modules/ledger/lib/analysis-range"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

interface LedgerAnalysisRangeControlProps {
  state: AnalysisRangeState
  // the dates the custom inputs start with: those the page shows data for
  prefill: LedgerAnalysisRange
  today: string
  onChange: (state: AnalysisRangeState) => void
}

// LedgerAnalysisRangeControl offers the presets and a custom from/to range.
// An invalid custom range shows why below the inputs.
export function LedgerAnalysisRangeControl({ state, prefill, today, onChange }: LedgerAnalysisRangeControlProps) {
  const { t } = useTranslation()
  const custom = state.kind === "custom"
  const problem = resolveRange(state, today).problem

  return (
    <div className="space-y-3">
      <div role="group" aria-label={t("ledger.analysis.range.label")} className="flex flex-wrap gap-2">
        {analysisPresets.map((preset) => {
          const active = state.kind === "preset" && state.preset === preset
          return (
            <Button
              key={preset}
              type="button"
              size="sm"
              variant={active ? "default" : "outline"}
              aria-pressed={active}
              onClick={() => onChange({ kind: "preset", preset })}
            >
              {t(`ledger.analysis.range.${preset}`)}
            </Button>
          )
        })}
        <Button
          type="button"
          size="sm"
          variant={custom ? "default" : "outline"}
          aria-pressed={custom}
          onClick={() => {
            if (!custom) onChange({ kind: "custom", from: prefill.from ?? "", to: prefill.to ?? "" })
          }}
        >
          {t("ledger.analysis.range.custom")}
        </Button>
      </div>
      {custom && (
        <div className="space-y-2">
          <div className="flex flex-wrap items-end gap-3">
            <div className="space-y-1">
              <Label htmlFor="ledger-analysis-from">{t("ledger.analysis.range.from")}</Label>
              <Input
                id="ledger-analysis-from"
                type="date"
                className="w-44"
                max={today}
                value={state.from}
                aria-invalid={problem !== undefined}
                onChange={(event) => onChange({ ...state, from: event.target.value })}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="ledger-analysis-to">{t("ledger.analysis.range.to")}</Label>
              <Input
                id="ledger-analysis-to"
                type="date"
                className="w-44"
                max={today}
                value={state.to}
                aria-invalid={problem !== undefined}
                onChange={(event) => onChange({ ...state, to: event.target.value })}
              />
            </div>
          </div>
          {problem && (
            <p role="alert" className="text-sm text-destructive">
              {t(`ledger.analysis.range.problems.${problem}`)}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
