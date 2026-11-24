import { useTranslation } from "react-i18next"
import {
  Bar,
  BarChart,
  CartesianGrid,
  ComposedChart,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"
import { AXIS_LINE, AXIS_TICK } from "@/components/charts/chart-axis"
import { ChartTooltipBox } from "@/components/charts/chart-parts"
import { formatDate } from "@/lib/format"
import { formatMonth } from "@/modules/ledger/lib/analysis-range"
import {
  chartRows,
  formatAmountCompact,
  seriesTooltipRows,
  spendingTooltipRows,
  type ChartSeries,
} from "@/modules/ledger/lib/analysis-series"

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
                rows={spendingTooltipRows(series, index, t("ledger.analysis.spending.total"))}
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

// CashFlowChart draws income and expenses side by side per month and the net
// result as a line; series are those of cashFlowSeries.
export function CashFlowChart({ months, series }: { months: string[]; series: ChartSeries[] }) {
  const { i18n } = useTranslation()
  const locale = i18n.language

  return (
    <ResponsiveContainer width="100%" height={280}>
      <ComposedChart data={chartRows(months, "month", series)} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
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
            return <ChartTooltipBox title={formatMonth(months[index], locale)} rows={seriesTooltipRows(series, index)} />
          }}
        />
        {series.map((s) =>
          s.key === "net" ? (
            <Line
              key={s.key}
              dataKey={s.key}
              name={s.label}
              stroke={s.color}
              strokeWidth={2}
              strokeLinecap="round"
              dot={{ r: 3, fill: s.color, stroke: "var(--card)", strokeWidth: 2 }}
              activeDot={{ r: 5 }}
              isAnimationActive={false}
            />
          ) : (
            <Bar key={s.key} dataKey={s.key} name={s.label} fill={s.color} maxBarSize={20} radius={[3, 3, 0, 0]} isAnimationActive={false} />
          ),
        )}
      </ComposedChart>
    </ResponsiveContainer>
  )
}

// NetWorthChart draws one line per series of netWorthSeries over the dates;
// the total is the thicker one.
export function NetWorthChart({ dates, series }: { dates: string[]; series: ChartSeries[] }) {
  const { i18n } = useTranslation()
  const locale = i18n.language

  return (
    <ResponsiveContainer width="100%" height={280}>
      <LineChart data={chartRows(dates, "date", series)} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
        <CartesianGrid vertical={false} stroke="var(--border)" />
        <XAxis dataKey="date" tick={AXIS_TICK} axisLine={AXIS_LINE} tickLine={false}
          tickFormatter={(date: string) => formatMonth(date.slice(0, 7), locale)} />
        <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={64}
          tickFormatter={(value: number) => formatAmountCompact(value, locale)} />
        <Tooltip
          content={({ active, label }) => {
            const index = dates.indexOf(String(label))
            if (!active || index < 0) return null
            return <ChartTooltipBox title={formatDate(dates[index])} rows={seriesTooltipRows(series, index)} />
          }}
        />
        {series.map((s) => (
          <Line
            key={s.key}
            dataKey={s.key}
            name={s.label}
            stroke={s.color}
            strokeWidth={s.key === "total" ? 2.5 : 1.5}
            strokeLinecap="round"
            dot={dates.length <= 24 ? { r: 3, fill: s.color, stroke: "var(--card)", strokeWidth: 2 } : false}
            activeDot={{ r: 5 }}
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
