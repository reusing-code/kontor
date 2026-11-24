// Chart pieces shared by the modules (axis styles: chart-axis.ts). This file
// must not import recharts itself. The bundler can still put it into the chunk
// that holds Recharts (its importers are chart pages), so a page without a
// chart has to check the production build before it imports from here.
//
// Rows and legend items are keyed by position: names come from user data
// (categories, accounts) and are not unique.

export interface TooltipRow {
  name: string
  value: string
  color?: string
}

export function ChartTooltipBox({ title, rows }: { title: string; rows: TooltipRow[] }) {
  return (
    <div className="rounded-md border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
      <div className="mb-1 font-medium">{title}</div>
      {rows.map((row, index) => (
        <div key={index} className="flex items-center gap-2">
          {row.color && <span className="h-2 w-2 rounded-[2px]" style={{ background: row.color }} />}
          <span className="text-muted-foreground">{row.name}</span>
          <span className="ml-auto pl-3 font-medium tabular-nums">{row.value}</span>
        </div>
      ))}
    </div>
  )
}

export interface LegendItem {
  label: string
  color: string
  // makes the entry a button
  onSelect?: () => void
}

export function ChartLegend({ items }: { items: LegendItem[] }) {
  return (
    <div className="mt-2 flex flex-wrap justify-center gap-x-4 gap-y-1">
      {items.map((item, index) => {
        const content = (
          <>
            <span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: item.color }} />
            {item.label}
          </>
        )
        return item.onSelect ? (
          <button key={index} type="button" onClick={item.onSelect}
            className="flex items-center gap-1.5 rounded-sm text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline focus-visible:outline-2">
            {content}
          </button>
        ) : (
          <div key={index} className="flex items-center gap-1.5 text-xs text-muted-foreground">
            {content}
          </div>
        )
      })}
    </div>
  )
}
