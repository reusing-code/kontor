import { useTranslation } from "react-i18next"
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"
import type { VehicleSummary } from "@/modules/auto/types"
import { chartCostTypes, costTypeColor, type ChartCostType } from "@/modules/auto/config/chart"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

const AXIS_TICK = { fill: "var(--muted-foreground)", fontSize: 12 }
const AXIS_LINE = { stroke: "var(--border)" }

interface TooltipRow {
  name: string
  value: string
  color?: string
}

function ChartTooltipBox({ title, rows }: { title: string; rows: TooltipRow[] }) {
  return (
    <div className="rounded-md border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
      <div className="mb-1 font-medium">{title}</div>
      {rows.map((row) => (
        <div key={row.name} className="flex items-center gap-2">
          {row.color && <span className="h-2 w-2 rounded-[2px]" style={{ background: row.color }} />}
          <span className="text-muted-foreground">{row.name}</span>
          <span className="ml-auto pl-3 font-medium tabular-nums">{row.value}</span>
        </div>
      ))}
    </div>
  )
}

function ChartLegend({ items }: { items: { label: string; color: string }[] }) {
  return (
    <div className="mt-2 flex flex-wrap justify-center gap-x-4 gap-y-1">
      {items.map((item) => (
        <div key={item.label} className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: item.color }} />
          {item.label}
        </div>
      ))}
    </div>
  )
}

interface StackSegmentProps {
  x?: number
  y?: number
  width?: number
  height?: number
  fill?: string
  dataKey?: string
  payload?: { year: number }
  topKeyByYear: Record<number, string>
}

// Stacked segments get a 2px surface gap below the segment above them; only the
// topmost non-zero segment carries the 4px rounded data-end.
function StackSegment(props: StackSegmentProps) {
  const { x = 0, y = 0, width = 0, height = 0, fill, dataKey, payload, topKeyByYear } = props
  if (height <= 0 || width <= 0) return null
  const isTop = payload && topKeyByYear[payload.year] === dataKey
  if (!isTop) {
    const h = Math.max(height - 2, 0)
    return <rect x={x} y={y + 2} width={width} height={h} fill={fill} />
  }
  const r = Math.min(4, height, width / 2)
  const d = `M ${x},${y + height}
    L ${x},${y + r}
    Q ${x},${y} ${x + r},${y}
    L ${x + width - r},${y}
    Q ${x + width},${y} ${x + width},${y + r}
    L ${x + width},${y + height} Z`
  return <path d={d} fill={fill} />
}

export function CostsByYearChart({ summary }: { summary: VehicleSummary }) {
  const { t } = useTranslation()
  const currency = t("common.currency")
  const data = summary.costsByYear
  if (data.length === 0) return null

  const topKeyByYear: Record<number, string> = {}
  for (const yc of data) {
    for (const type of chartCostTypes) {
      if (yc[type] > 0) topKeyByYear[yc.year] = type
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("vehicleSummary.costsByYear")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ResponsiveContainer width="100%" height={280}>
          <BarChart data={data} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <CartesianGrid vertical={false} stroke="var(--border)" />
            <XAxis dataKey="year" tick={AXIS_TICK} axisLine={AXIS_LINE} tickLine={false} />
            <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={56}
              tickFormatter={(v: number) => v.toLocaleString()} />
            <Tooltip
              cursor={{ fill: "var(--muted)", opacity: 0.4 }}
              content={({ active, payload, label }) => {
                if (!active || !payload?.length) return null
                const rows = payload
                  .filter((p) => typeof p.value === "number" && p.value > 0)
                  .reverse()
                  .map((p) => ({
                    name: t(`costTypes.${p.dataKey}`),
                    value: `${(p.value as number).toFixed(2)} ${currency}`,
                    color: costTypeColor(p.dataKey as ChartCostType),
                  }))
                const total = payload.reduce((sum, p) => sum + ((p.value as number) || 0), 0)
                rows.push({ name: t("vehicleSummary.total"), value: `${total.toFixed(2)} ${currency}`, color: "" })
                return <ChartTooltipBox title={String(label)} rows={rows} />
              }}
            />
            <Legend content={() => (
              <ChartLegend items={chartCostTypes.map((type) => ({
                label: t(`costTypes.${type}`),
                color: costTypeColor(type),
              }))} />
            )} />
            {chartCostTypes.map((type) => (
              <Bar
                key={type}
                dataKey={type}
                stackId="costs"
                fill={costTypeColor(type)}
                maxBarSize={24}
                isAnimationActive={false}
                shape={(props: unknown) => (
                  <StackSegment {...(props as StackSegmentProps)} topKeyByYear={topKeyByYear} />
                )}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </CardContent>
    </Card>
  )
}

export function CostPerKmChart({ summary }: { summary: VehicleSummary }) {
  const { t } = useTranslation()
  const currency = t("common.currency")
  const kmByYear = new Map(summary.mileageByYear.map((ym) => [ym.year, ym]))
  const data = summary.costsByYear
    .map((yc) => {
      const ym = kmByYear.get(yc.year)
      return {
        year: yc.year,
        total: ym && ym.km > 0 ? yc.total / ym.km : null,
        fuel: ym && ym.fuelCostPerKm > 0 ? ym.fuelCostPerKm : null,
      }
    })
    .filter((row) => row.total != null || row.fuel != null)
  if (data.length === 0) return null

  const series = [
    { key: "total", label: t("vehicleSummary.total"), color: "var(--foreground)" },
    { key: "fuel", label: t("costTypes.fuel"), color: costTypeColor("fuel") },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("vehicleSummary.costPerKmByYear")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ResponsiveContainer width="100%" height={240}>
          <LineChart data={data} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <CartesianGrid vertical={false} stroke="var(--border)" />
            <XAxis dataKey="year" tick={AXIS_TICK} axisLine={AXIS_LINE} tickLine={false} />
            <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={48}
              tickFormatter={(v: number) => v.toFixed(2)} />
            <Tooltip
              content={({ active, payload, label }) => {
                if (!active || !payload?.length) return null
                const rows = payload
                  .filter((p) => typeof p.value === "number")
                  .map((p) => {
                    const s = series.find((sr) => sr.key === p.dataKey)
                    return {
                      name: s?.label ?? String(p.dataKey),
                      value: `${(p.value as number).toFixed(2)} ${currency}/km`,
                      color: s?.color,
                    }
                  })
                return <ChartTooltipBox title={String(label)} rows={rows} />
              }}
            />
            <Legend content={() => <ChartLegend items={series} />} />
            {series.map((s) => (
              <Line
                key={s.key}
                dataKey={s.key}
                stroke={s.color}
                strokeWidth={2}
                strokeLinecap="round"
                connectNulls
                dot={{ r: 4, fill: s.color, stroke: "var(--card)", strokeWidth: 2 }}
                activeDot={{ r: 5 }}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </CardContent>
    </Card>
  )
}

export function KmPerYearChart({ summary }: { summary: VehicleSummary }) {
  const { t } = useTranslation()
  const data = summary.mileageByYear
  if (data.length === 0) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("vehicleSummary.kmPerYear")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ResponsiveContainer width="100%" height={240}>
          <BarChart data={data} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <CartesianGrid vertical={false} stroke="var(--border)" />
            <XAxis dataKey="year" tick={AXIS_TICK} axisLine={AXIS_LINE} tickLine={false} />
            <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={56}
              tickFormatter={(v: number) => v.toLocaleString()} />
            <Tooltip
              cursor={{ fill: "var(--muted)", opacity: 0.4 }}
              content={({ active, payload, label }) => {
                if (!active || !payload?.length) return null
                const km = payload[0].value as number
                return (
                  <ChartTooltipBox
                    title={String(label)}
                    rows={[{ name: t("vehicleSummary.km"), value: `${Math.round(km).toLocaleString()} km` }]}
                  />
                )
              }}
            />
            <Bar
              dataKey="km"
              fill="var(--viz-service)"
              maxBarSize={24}
              radius={[4, 4, 0, 0]}
              isAnimationActive={false}
            />
          </BarChart>
        </ResponsiveContainer>
      </CardContent>
    </Card>
  )
}

export function MileageHistoryChart({ summary }: { summary: VehicleSummary }) {
  const { t } = useTranslation()
  const data = summary.mileageHistory
    .map((p) => ({ ts: new Date(p.date).getTime(), mileage: p.mileage }))
    .filter((p) => !Number.isNaN(p.ts))
    .sort((a, b) => a.ts - b.ts)
  if (data.length < 2) return null

  const fmtDate = (ts: number) => new Date(ts).toLocaleDateString()

  const firstYear = new Date(data[0].ts).getFullYear() + 1
  const lastYear = new Date(data[data.length - 1].ts).getFullYear()
  const yearTicks: number[] = []
  for (let y = firstYear; y <= lastYear; y++) {
    yearTicks.push(Date.UTC(y, 0, 1))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("vehicleSummary.mileageHistory")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ResponsiveContainer width="100%" height={240}>
          <LineChart data={data} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <CartesianGrid vertical={false} stroke="var(--border)" />
            <XAxis
              dataKey="ts"
              type="number"
              scale="time"
              domain={["dataMin", "dataMax"]}
              ticks={yearTicks}
              tick={AXIS_TICK}
              axisLine={AXIS_LINE}
              tickLine={false}
              tickFormatter={(ts: number) => String(new Date(ts).getFullYear())}
            />
            <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={64}
              tickFormatter={(v: number) => v.toLocaleString()} />
            <Tooltip
              content={({ active, payload }) => {
                if (!active || !payload?.length) return null
                const point = payload[0].payload as { ts: number; mileage: number }
                return (
                  <ChartTooltipBox
                    title={fmtDate(point.ts)}
                    rows={[{
                      name: t("vehicleSummary.currentMileage"),
                      value: `${Math.round(point.mileage).toLocaleString()} km`,
                    }]}
                  />
                )
              }}
            />
            <Line
              dataKey="mileage"
              stroke="var(--viz-service)"
              strokeWidth={2}
              strokeLinecap="round"
              dot={{ r: 4, fill: "var(--viz-service)", stroke: "var(--card)", strokeWidth: 2 }}
              activeDot={{ r: 5 }}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </CardContent>
    </Card>
  )
}
