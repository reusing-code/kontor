import { ApiError } from "@/lib/api"

const errorCodes = ["imap_login_failed", "imap_unavailable", "password_missing"] as const

export function folderErrorKey(error: unknown) {
  const code = error instanceof ApiError ? errorCodes.find((c) => c === error.code) : undefined
  return `ledger.email.foldersErrors.${code ?? "generic"}` as const
}

// The folders that match what is typed; all of them while the field is empty
// or holds one of the listed names, so the other choices stay visible.
export function visibleFolders(folders: string[], value: string): string[] {
  const query = value.trim().toLowerCase()
  if (query === "" || folders.some((folder) => folder.toLowerCase() === query)) return folders
  return folders.filter((folder) => folder.toLowerCase().includes(query))
}
