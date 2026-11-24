import { beforeEach, describe, expect, it, vi } from "vitest"
import { ApiError, USER_KEY, del, download, get, getToken, post, postForm, sendJson, setToken } from "@/lib/api"

function respond(status: number, body?: unknown, headers: Record<string, string> = {}) {
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
    new Response(body === undefined ? null : JSON.stringify(body), { status, headers }),
  )
  vi.stubGlobal("fetch", fetchMock)
  return fetchMock
}

describe("send", () => {
  let location: { href: string }

  beforeEach(() => {
    location = { href: "/settings" }
    vi.stubGlobal("location", location)
    setToken("jwt")
    localStorage.setItem(USER_KEY, JSON.stringify({ id: "u1" }))
  })

  it("sends the token and a JSON body", async () => {
    const fetchMock = respond(200, { id: "1" })

    await expect(post("/things", { name: "a" })).resolves.toEqual({ id: "1" })

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe("/api/v1/things")
    expect(init?.method).toBe("POST")
    expect(init?.headers).toEqual({ "Content-Type": "application/json", Authorization: "Bearer jwt" })
    expect(init?.body).toBe(JSON.stringify({ name: "a" }))
  })

  it("sends form data without a JSON content type", async () => {
    const fetchMock = respond(200, { ok: true })
    const form = new FormData()
    form.append("file", "x")

    await postForm("/upload", form)

    const init = fetchMock.mock.calls[0][1]
    expect(init?.headers).toEqual({ Authorization: "Bearer jwt" })
    expect(init?.body).toBe(form)
  })

  it("ends the session on a 401 and redirects to the login", async () => {
    respond(401, { error: "token expired" })

    const error = await get("/things").catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).status).toBe(401)
    expect(getToken()).toBeNull()
    expect(localStorage.getItem(USER_KEY)).toBeNull()
    expect(location.href).toBe("/login")
  })

  it("treats a 401 as an ordinary error when the redirect is opted out", async () => {
    respond(401, { error: "invalid credentials" })

    const error = await sendJson("POST", "/auth/login", { json: {}, redirectOnUnauthorized: false }).catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).message).toBe("invalid credentials")
    expect(getToken()).toBe("jwt")
    expect(location.href).toBe("/settings")
  })

  it("carries the error code and body of a failed request", async () => {
    respond(400, { error: "not allowed", code: "redirect_url_not_allowed", redirectUrl: "https://x" })

    const error = (await get("/things").catch((e: unknown) => e)) as ApiError

    expect(error.status).toBe(400)
    expect(error.message).toBe("not allowed")
    expect(error.code).toBe("redirect_url_not_allowed")
    expect(error.body.redirectUrl).toBe("https://x")
  })

  it("falls back to the status text for an error without a JSON body", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("oops", { status: 502, statusText: "Bad Gateway" })))

    const error = (await get("/things").catch((e: unknown) => e)) as ApiError

    expect(error.message).toBe("Bad Gateway")
    expect(error.code).toBeUndefined()
  })

  it("returns undefined for a 204", async () => {
    respond(204)

    await expect(sendJson("PUT", "/things/1", { json: {} })).resolves.toBeUndefined()
    await expect(del("/things/1")).resolves.toBeUndefined()
  })

  it("downloads with the file name of the response", async () => {
    respond(200, { data: 1 }, { "Content-Disposition": 'attachment; filename="export.json"' })
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: vi.fn(() => "blob:1"), revokeObjectURL: vi.fn() }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {})
    const created: HTMLAnchorElement[] = []
    const createElement = document.createElement.bind(document)
    vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
      const el = createElement(tag)
      if (el instanceof HTMLAnchorElement) created.push(el)
      return el
    })

    await download("/export", "fallback.json")

    expect(click).toHaveBeenCalledOnce()
    expect(created[0].download).toBe("export.json")
  })
})
