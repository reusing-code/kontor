import "@testing-library/jest-dom/vitest"
import { afterEach, beforeAll, vi } from "vitest"
import { cleanup } from "@testing-library/react"
import { toast } from "sonner"
import i18n from "@/i18n"

// Radix Select, Dialog and Checkbox use these; jsdom has none of them.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub
const elementStubs = {
  scrollIntoView: () => {},
  hasPointerCapture: () => false,
  setPointerCapture: () => {},
  releasePointerCapture: () => {},
}
for (const [name, value] of Object.entries(elementStubs)) {
  if (!(name in Element.prototype)) {
    Object.defineProperty(Element.prototype, name, { value, configurable: true, writable: true })
  }
}
window.matchMedia ??= (query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
  dispatchEvent: () => false,
})

beforeAll(async () => {
  await i18n.changeLanguage("en")
})

afterEach(() => {
  // sonner keeps its toasts in module state, across unmounts
  toast.dismiss()
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
  localStorage.clear()
})
