import { vi } from "vitest"

// jsdom lays nothing out, so Recharts' ResponsiveContainer measures 0x0 and
// renders no chart. stubChartSize gives every element a fixed box for one
// test (restoreMocks in vitest.config.ts undoes it afterwards).
export function stubChartSize(width = 800, height = 300) {
  return vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width,
    height,
    top: 0,
    left: 0,
    right: width,
    bottom: height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  })
}
