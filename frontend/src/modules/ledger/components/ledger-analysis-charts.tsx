import { useTranslation } from "react-i18next"
import { Bar, BarChart, CartesianGrid, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { AXIS_LINE, AXIS_TICK } from "@/components/charts/chart-axis"
import { ChartTooltipBox } from "@/components/charts/chart-parts"
import { formatMonth } from "@/modules/ledger/lib/analysis-range"
import { chartRows, formatAmountCompact, spendingTooltipRows, type ChartSeries } from "@/modules/ledger/lib/analysis-series"

// The only file of the ledger module that imports recharts: everything else
// (helpers, legends, the dashboard) stays out of the chart chunk.

// SpendingChart stacks the series per month; negative values (a subcategory
// with a net inflow) are drawn below the axis.
export function SpendingChart({ months, series }: { months: string[]; series: ChartSeries[] }) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language

  return (
    <ResponsiveContainer width="100%" height={280}>
      <BarChart data={chartRows(months, "month", series)} stackOffset="sign" margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
        <CartesianGrid vertical={false} stroke="var(--border)" />
        <XAxis dataKey="month" tick={AXIS_TICK} axisLine={AXIS_LINE} tickLine={false}
          tickFormatter={(month: string) => formatMonth(month, locale)} />
        <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={64}
          tickFormatter={(value: number) => formatAmountCompact(value, locale)} />
        <ReferenceLine y={0} stroke="var(--border)" />
        <Tooltip
          cursor={{ fill: "var(--muted)", opacity: 0.4 }}
          content={({ active, label }) => {
            const index = months.indexOf(String(label))
            if (!active || index < 0) return null
            return (
              <ChartTooltipBox
                title={formatMonth(months[index], locale)}
                rows={spendingTooltipRows(series, index, t("ledger.analysis.spending.total"), locale)}
              />
            )
          }}
        />
        {series.map((s) => (
          <Bar key={s.key} dataKey={s.key} name={s.label} stackId="spending" fill={s.color} maxBarSize={32} isAnimationActive={false} />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}
