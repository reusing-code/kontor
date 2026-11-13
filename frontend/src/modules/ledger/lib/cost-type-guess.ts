import type { CostType } from "@/modules/auto/types"

// words match only as whole words (short brand names would hit inside other
// words), parts anywhere in the text. Earlier rules win: "kfz-steuer" is a
// tax, not a workshop, and a tire shop of a fuel brand is still about tires.
const rules: { type: CostType; words: string[]; parts: string[] }[] = [
  { type: "tax", words: [], parts: ["kfz-steuer", "kfz steuer", "kraftfahrzeugsteuer", "bundeskasse", "hauptzollamt"] },
  { type: "insurance", words: ["huk", "devk"], parts: ["versicherung", "insurance", "allianz", "cosmos"] },
  { type: "tires", words: ["tyre", "tyres", "tire", "tires", "point s"], parts: ["reifen", "vergölst"] },
  {
    type: "fuel",
    words: ["aral", "esso", "jet", "agip", "eni", "avia", "omv", "tank", "star", "hem", "total", "westfalen", "raiffeisen"],
    parts: ["shell", "totalenergies", "tankstelle", "oil!"],
  },
  { type: "service", words: ["atu", "kfz", "tüv", "tuev"], parts: ["werkstatt", "autohaus", "a.t.u", "pitstop", "dekra", "inspektion"] },
]

function escape(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

const matchers = rules.map(({ type, words, parts }) => ({
  type,
  pattern: new RegExp(
    [...words.map((word) => `(?<![\\p{L}\\p{N}])${escape(word)}(?![\\p{L}\\p{N}])`), ...parts.map(escape)].join("|"),
    "iu",
  ),
}))

export function guessCostType(...texts: (string | undefined)[]): CostType {
  const text = texts.filter(Boolean).join(" ")
  return matchers.find(({ pattern }) => pattern.test(text))?.type ?? "misc"
}
