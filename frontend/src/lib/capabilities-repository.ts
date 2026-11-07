import { get } from "@/lib/api"
import type { Capabilities } from "@/types/capabilities"

export async function getCapabilities(): Promise<Capabilities> {
  return get<Capabilities>("/capabilities")
}
