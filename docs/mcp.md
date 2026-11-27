# Using Kontor from an agent

In **Settings → Access tokens**, create a token for your client. Select only the scopes it needs and set an expiry (90 days by default). Copy the token when it is shown; Kontor stores only its hash. The same screen offers ready-to-copy client configurations.

Connect the client to `https://<your-kontor>/mcp` over streamable HTTP with the header `Authorization: Bearer <token>`. This endpoint accepts personal access tokens, not login JWTs. For example, with Claude Code:

```sh
claude mcp add --transport http kontor https://kontor.example.com/mcp --header "Authorization: Bearer <your-token>"
```

For OpenCode, add the following to `opencode.json`:

```json
{
  "mcp": {
    "kontor": {
      "type": "remote",
      "url": "https://kontor.example.com/mcp",
      "headers": { "Authorization": "Bearer <your-token>" }
    }
  }
}
```

## Token scopes

| Scope | Through MCP | Through the [REST API](api.md) |
| --- | --- | --- |
| `read` | List reference data and search records and paperless documents. | Read routes, including the full data export. Token management and the bank list require a login JWT. |
| `drafts` | Propose a contract, purchase, vehicle, cost, category, or ledger account in the inbox. | Edit, accept, or reject individual pending drafts, including drafts from other sources. Bulk acceptance requires a login JWT. |
| `links` | Attach paperless documents to existing entries and add references to ledger transactions. | Attach documents and add transaction references. |
| `categorize` | Directly change ledger category names, hierarchy, and match words; set transaction categories and link or unlink transfers. | No access through this scope. |

MCP tools only appear for enabled modules and authorized scopes. There is no MCP tool to accept a draft or create an entity directly. Linking and categorization change existing data without inbox review. To attach paperless documents, set `PUBLIC_URL` on the server: MCP checks that the entity exists and derives the back-link from that origin.

**The same token works on REST, where some permissions are broader.** A `drafts` token can decide its own drafts without review and accept with an unverified entity ID. For a paperless draft this can write a back-link and change tags; for an email draft it can trigger the configured mailbox action. A `links` token on REST supplies the paperless back-link URL without verifying the entity exists. Give each client its own token and revoke it in Settings if it is exposed; changing the login password does not revoke tokens.
