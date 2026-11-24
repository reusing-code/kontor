import { describe, expect, it } from "vitest"
import { guessCostType } from "@/modules/ledger/lib/cost-type-guess"

describe("guessCostType", () => {
  it.each([
    ["tax", "Bundeskasse in Trier", "KFZ-STEUER B-XY 123"],
    ["tax", "Hauptzollamt Hamburg", undefined],
    ["tax", undefined, "Kraftfahrzeugsteuer 2026"],
    ["insurance", "HUK-COBURG", "Beitrag 2026"],
    ["insurance", "Allianz Versicherungs-AG", undefined],
    ["insurance", "CosmosDirekt", undefined],
    ["insurance", "DEVK", undefined],
    ["tires", "Reifen Müller GmbH", undefined],
    ["tires", "Vergölst", undefined],
    ["tires", "Point S Autoservice", undefined],
    ["tires", undefined, "Winter tyres"],
    ["fuel", "ARAL Station 1234", undefined],
    ["fuel", "Shell Deutschland", undefined],
    ["fuel", "JET Tankstelle", undefined],
    ["fuel", "TotalEnergies", undefined],
    ["fuel", "Westfalen AG", undefined],
    ["service", "A.T.U Auto-Teile-Unger", undefined],
    ["service", "Autohaus Schmidt", "Inspektion"],
    ["service", "TÜV Süd", undefined],
    ["service", "DEKRA", "Hauptuntersuchung"],
    ["service", "Kfz Meier", undefined],
  ] as const)("finds %s in %s / %s", (type, counterparty, purpose) => {
    expect(guessCostType(counterparty, purpose)).toBe(type)
  })

  it("searches counterparty, purpose and the order merchant", () => {
    expect(guessCostType("PayPal Europe", "Ihre Bestellung", "reifen.com")).toBe("tires")
    expect(guessCostType("PayPal Europe", "Aral Fuel&Go")).toBe("fuel")
    expect(guessCostType(undefined, undefined, "Allianz Direct")).toBe("insurance")
  })

  it("matches regardless of case", () => {
    expect(guessCostType("aral")).toBe("fuel")
    expect(guessCostType("VERSICHERUNG")).toBe("insurance")
  })

  it("matches short brand names only as whole words", () => {
    expect(guessCostType("Esso")).toBe("fuel")
    expect(guessCostType("Bäckerei Jetzinger")).toBe("misc")
    expect(guessCostType("Starbucks")).toBe("misc")
    expect(guessCostType("Hemden-Shop")).toBe("misc")
    expect(guessCostType("Batu Restaurant")).toBe("misc")
    expect(guessCostType("Thukral")).toBe("misc")
    expect(guessCostType("Kfzteile24")).toBe("misc")
  })

  // Gap: a digit counts as a word character, so the insurer's online brand HUK24 is no whole-word "huk".
  it.fails("finds a short brand name followed by digits", () => {
    expect(guessCostType("HUK24 AG")).toBe("insurance")
  })

  it("falls back to misc without a match or without text", () => {
    expect(guessCostType("Amazon", "Bestellung 123")).toBe("misc")
    expect(guessCostType()).toBe("misc")
    expect(guessCostType(undefined, "")).toBe("misc")
  })

  it("lets the earlier rule win when several match", () => {
    // tax, insurance, tires, fuel, service
    expect(guessCostType("Bundeskasse", "Kfz-Steuer")).toBe("tax")
    expect(guessCostType("Kfz Versicherung")).toBe("insurance")
    expect(guessCostType("Reifen Center an der Aral")).toBe("tires")
    expect(guessCostType("Shell", "Werkstatt")).toBe("fuel")
  })
})
