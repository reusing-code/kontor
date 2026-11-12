export type TokenScope = "read" | "drafts" | "links"

export const tokenScopes: TokenScope[] = ["read", "drafts", "links"]

export interface Token {
  id: string
  name: string
  scopes: TokenScope[]
  createdAt: string
  expiresAt?: string
  lastUsedAt?: string
}

export interface TokenCreateInput {
  name: string
  scopes: TokenScope[]
  expiresAt?: string
}

// The raw token only exists in the create response.
export interface TokenCreated extends Token {
  token: string
}
