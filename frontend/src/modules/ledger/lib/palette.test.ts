import { describe, expect, it } from "vitest"

// The stylesheet is read from disk, relative to this file: the test config
// turns CSS imports into nothing. tsconfig.app.json has no Node types (the app
// must not use Node), so the module is loaded by a computed name and typed here.
const fsModule = `node:${"fs"}`
const fs = (await import(/* @vite-ignore */ fsModule)) as { readFileSync(path: URL, encoding: "utf8"): string }
// (not written as new URL(…, import.meta.url): Vite would turn that into an asset URL)
const here = import.meta.url
const css = fs.readFileSync(new URL("../../../index.css", here), "utf8")

// The light-mode series colors of the ledger charts, read from index.css.
const checked = [
  ...Array.from({ length: 10 }, (_, i) => `--viz-cat-${i + 1}`),
  "--viz-cat-other",
  "--viz-cat-uncategorized",
  "--viz-income",
  "--viz-expenses",
]

function lightValues(): Record<string, string> {
  const values: Record<string, string> = {}
  // the first definition of a variable is the one of :root (light)
  for (const [, name, value] of css.matchAll(/(--viz-[a-z0-9-]+):\s*(#[0-9a-f]{6});/g)) {
    values[name] ??= value
  }
  return values
}

// The values of the .dark block that follows the light category colors.
function darkValues(): Record<string, string> {
  const at = css.indexOf(".dark {", css.indexOf("--viz-cat-1:"))
  const block = css.slice(at, css.indexOf("}", at))
  return Object.fromEntries([...block.matchAll(/(--viz-[a-z0-9-]+):\s*(#[0-9a-f]{6});/g)].map(([, name, value]) => [name, value]))
}

const channels = (hex: string) => [1, 3, 5].map((at) => parseInt(hex.slice(at, at + 2), 16))

// WCAG relative luminance and contrast ratio against white.
function contrastOnWhite(hex: string): number {
  const [r, g, b] = channels(hex).map((c) => {
    const s = c / 255
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 1.05 / (0.2126 * r + 0.7152 * g + 0.0722 * b + 0.05)
}

function distance(a: string, b: string): number {
  const [x, y] = [channels(a), channels(b)]
  return Math.hypot(x[0] - y[0], x[1] - y[1], x[2] - y[2])
}

describe("ledger chart palette (light)", () => {
  const values = lightValues()

  it("defines every checked color", () => {
    for (const name of checked) expect(values[name], name).toMatch(/^#[0-9a-f]{6}$/)
  })

  it("has at least 3:1 contrast against the white card", () => {
    for (const name of checked) expect(contrastOnWhite(values[name]), `${name} ${values[name]}`).toBeGreaterThanOrEqual(3)
  })

  it("keeps the colors apart", () => {
    const names = checked.filter((name) => name.startsWith("--viz-cat-"))
    for (const a of names) {
      for (const b of names) {
        if (a < b) expect(distance(values[a], values[b]), `${a} ${b}`).toBeGreaterThanOrEqual(40)
      }
    }
  })
})

// Each palette color has three tints for the subcategories of the two-ring
// chart; a parent's children cycle through the color and its tints, so every
// step has to be told apart from the next.
describe("ledger chart palette tints", () => {
  for (const [mode, values] of [["light", lightValues()], ["dark", darkValues()]] as const) {
    it(`defines three distinct tints per category color (${mode})`, () => {
      for (let n = 1; n <= 10; n++) {
        const steps = [`--viz-cat-${n}`, ...[1, 2, 3].map((step) => `--viz-cat-${n}-tint-${step}`)]
        for (const name of steps) expect(values[name], `${mode} ${name}`).toMatch(/^#[0-9a-f]{6}$/)
        for (let i = 0; i + 1 < steps.length; i++) {
          expect(distance(values[steps[i]], values[steps[i + 1]]), `${mode} ${steps[i]} ${steps[i + 1]}`).toBeGreaterThanOrEqual(25)
        }
      }
    })
  }
})
