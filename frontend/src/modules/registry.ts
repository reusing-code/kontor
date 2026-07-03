import type { ModuleDefinition, ModuleId } from "@/types/modules"
import { contractsModule } from "./contracts"
import { purchasesModule } from "./purchases"
import { autoModule } from "./auto"
import { ledgerModule } from "./ledger"

export const modules: ModuleDefinition[] = [contractsModule, purchasesModule, autoModule, ledgerModule]

export function getModule(id: ModuleId): ModuleDefinition {
  const module = modules.find((m) => m.id === id)
  if (!module) {
    throw new Error(`Unknown module: ${id}`)
  }
  return module
}
