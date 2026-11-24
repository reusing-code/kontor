import type { z } from "zod/v4"

// The paths of the issues a schema reports for input, dot-joined; empty when it parses.
export function issuePaths(schema: z.ZodType, input: unknown): string[] {
  const result = schema.safeParse(input)
  return result.success ? [] : result.error.issues.map((issue) => issue.path.join("."))
}

// Path and message of each issue, for refinements with a translated message key.
export function issues(schema: z.ZodType, input: unknown): { path: string; message: string }[] {
  const result = schema.safeParse(input)
  return result.success ? [] : result.error.issues.map((issue) => ({ path: issue.path.join("."), message: issue.message }))
}
