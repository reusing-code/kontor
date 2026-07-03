export const chartCostTypes = ["service", "fuel", "insurance", "tax", "tires", "misc"] as const
export type ChartCostType = (typeof chartCostTypes)[number]

export const costTypeColor = (type: ChartCostType) => `var(--viz-${type})`
