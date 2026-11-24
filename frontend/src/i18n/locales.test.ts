import { describe, expect, it } from "vitest"
import de from "./locales/de.json"
import en from "./locales/en.json"

type Messages = { [key: string]: string | Messages }

function flatten(messages: Messages, prefix = ""): Map<string, string> {
  return new Map(
    Object.entries(messages).flatMap(([key, value]) =>
      typeof value === "string" ? [[prefix + key, value] as const] : [...flatten(value, `${prefix}${key}.`)],
    ),
  )
}

const locales = { en: flatten(en), de: flatten(de) }

const missingIn = (from: Map<string, string>, to: Map<string, string>) => [...from.keys()].filter((key) => !to.has(key)).sort()

const sources = import.meta.glob<string>(["/src/**/*.{ts,tsx}", "!/src/**/*.test.{ts,tsx}"], { query: "?raw", import: "default", eager: true })

describe("locales", () => {
  it("have the same keys in English and German", () => {
    expect({ missingInDe: missingIn(locales.en, locales.de), missingInEn: missingIn(locales.de, locales.en) }).toEqual({
      missingInDe: [],
      missingInEn: [],
    })
  })

  it("have no empty texts", () => {
    const empty = Object.entries(locales).flatMap(([lang, messages]) =>
      [...messages].filter(([, value]) => value.trim() === "").map(([key]) => `${lang}:${key}`),
    )
    expect(empty).toEqual([])
  })

  // Only literal keys (t("a.b")); keys built from template literals are not checked.
  it("contain every literal key the code translates", () => {
    const plurals = new Set([...locales.en.keys()].map((key) => key.replace(/_(zero|one|two|few|many|other)$/, "")))
    const used = Object.values(sources).flatMap((source) => [...source.matchAll(/\bt\(\s*"([^"]+)"/g)].map((match) => match[1]))
    expect(used.length).toBeGreaterThan(100)
    const unknown = [...new Set(used)].filter((key) => !locales.en.has(key) && !plurals.has(key)).sort()
    expect(unknown).toEqual([])
  })
})
