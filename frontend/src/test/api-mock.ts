import { vi } from "vitest"

export interface ApiCall {
  method: string
  // Path below /api/v1, including the query string.
  path: string
  body: unknown
  // The body of a multipart request.
  form?: FormData
}

class ApiFailure {
  status: number
  message: string
  code?: string

  constructor(status: number, message: string, code?: string) {
    this.status = status
    this.message = message
    this.code = code
  }
}

export function apiError(status: number, message: string, code?: string): ApiFailure {
  return new ApiFailure(status, message, code)
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

// mockApi replaces fetch with a router over "METHOD /path" keys (path below
// /api/v1; a key without query string matches any query). A handler is the
// response body or a function of the ApiCall returning it (or a promise of
// it); undefined answers 204 and apiError(...) an error response. Unknown
// routes answer 404.
export function mockApi(routes: Record<string, unknown>) {
  const calls: ApiCall[] = []

  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(typeof input === "string" || input instanceof URL ? input : input.url, window.location.origin)
    const method = (init?.method ?? "GET").toUpperCase()
    const path = url.pathname.replace(/^\/api\/v1/, "")
    const call: ApiCall = {
      method,
      path: path + url.search,
      body: typeof init?.body === "string" ? JSON.parse(init.body) : undefined,
      ...(init?.body instanceof FormData ? { form: init.body } : {}),
    }
    calls.push(call)

    const key = [`${method} ${call.path}`, `${method} ${path}`].find((k) => k in routes)
    if (!key) return json(404, { error: `no mock for ${method} ${call.path}` })
    const handler = routes[key]
    const result: unknown = await (typeof handler === "function" ? (handler as (call: ApiCall) => unknown)(call) : handler)
    if (result instanceof ApiFailure) return json(result.status, { error: result.message, ...(result.code ? { code: result.code } : {}) })
    if (result === undefined) return new Response(null, { status: 204 })
    return json(200, result)
  })
  vi.stubGlobal("fetch", fetchMock)

  return {
    calls,
    routes,
    callsTo: (method: string, path: string) => calls.filter((c) => c.method === method && c.path === path),
  }
}
