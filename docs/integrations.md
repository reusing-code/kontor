# Integrations and import workflows

Contracts, purchases, vehicles, CSV imports, and ledger analysis work without an external service. Choose an integration based on what you want to bring into Kontor.

## Bank statements and recurring payments

Import a DKB or comdirect CSV for your transaction history. The preview shows the balance if the file includes one. For automatic updates, [connect a bank through Enable Banking](bank-import.md) **after** importing CSV history so the sync can recognize transactions you already have.

The review queue groups payments by counterparty and proposed category. Accept a group together, split a payment across categories, or link transfers between your accounts so they don’t count as spending. Kontor automatically links unambiguous transfer pairs. Account balances can also be set by hand; a drift warning indicates that two reported balances don’t agree with the transactions between them.

After an import, Kontor looks for outgoing EUR payments to the same counterparty on a monthly, quarterly, or yearly schedule. It tolerates small changes in price (10% or €1, whichever is larger). A match becomes a proposed contract in the inbox; choose its category and accept to link the payments. A rejected group isn’t proposed again. This detection uses rules and needs no model.

## Paperless-ngx and receipts

Set `ENCRYPTION_KEY` on the server, then connect your paperless-ngx URL and API token in **Settings**. Search your archive from Kontor and attach documents to entries. Kontor stores the link and writes a back-link to the document’s **Kontor** URL field. A paperclip in lists shows which entries have documents. Manual attachment works without an LLM.

With an LLM configured, enable the draft pipeline in Settings and tag a document `kontor-inbox` in paperless. Kontor reads its OCR text and proposes a purchase, contract, or vehicle cost. Accepting creates the entry, links the document, and replaces the inbox tag with `kontor`; rejecting removes the inbox tag. Both tag names are configurable. The server checks every `PAPERLESS_PIPELINE_INTERVAL` (default `15m`; `0` disables polling).

In the inbox, review one draft in a prefilled form or select several to accept as they are. A draft that fails validation stays pending with its reason; retrying a batch doesn’t duplicate entries.

## Optional language model

Set `LLM_BASE_URL` and `LLM_MODEL` for an OpenAI-compatible endpoint. `LLM_API_KEY` is optional; `LLM_TIMEOUT` defaults to `60s`. The endpoint is shared across users. Leave `LLM_BASE_URL` unset to keep model-backed features off. Prompts and responses aren’t logged.

- **Quick add:** Press Ctrl/Cmd+K and type a note or attach an invoice. Kontor suggests a prefilled purchase, contract, or vehicle cost. PDFs need a text layer; photos (JPEG, PNG, WebP) require `LLM_VISION_MODEL` on the same endpoint. Files are limited to 10 MiB. Kontor never stores the attachment and only creates the entry when you save it.
- **Category suggestions:** After an import, matching rules run first. For the remaining uncategorized payments, the model sees the date, amount, counterparty, purpose, category tree, and a few reviewed examples. Accept suggestions in the review queue and optionally save a match word as a rule. `LEDGER_SUGGEST_BATCH_SIZE` defaults to `25` and `LEDGER_SUGGEST_MAX_PER_RUN` to `100`.
- **Foreign currencies:** Extracted entries use EUR. Kontor looks up the ECB reference rate via the Frankfurter API and keeps the original amount in the comments. Without a rate it leaves the amount empty. `FX_RATES_URL` selects the endpoint (`off` disables it). Email orders keep their original currency.

## Email orders and drafts

With `ENCRYPTION_KEY` and the LLM configured, add an IMAP account under **Ledger → Email accounts**. Choose a folder (or Gmail label) for each workflow:

| Folder | What happens |
| --- | --- |
| Order emails | Kontor extracts the merchant, items, total, date, payment method, and order ID, then looks for the corresponding bank payment. You can also upload `.eml` files. For a payment in another currency, it links automatically only if exactly one candidate is within 5% of the converted total. |
| Kontor inbox (optional, separate folder) | File contract confirmations and bills here to receive drafts. An accepted entry links to its source email; a rejected message is remembered and won’t be proposed again. |

For extraction, only the sender, subject, date, and text body from the chosen folder go to the model. Optional order pre-selection sends only subjects and senders first. Known messages are skipped. Scans run on demand or every `LEDGER_EMAIL_SCAN_INTERVAL` (default `6h`); a timed-out scan or one interrupted by repeated LLM failures resumes after about five minutes with scheduling enabled. After `LEDGER_EMAIL_MAX_EXTRACTION_FAILURES` unusable answers (default `3`), an order message is ignored.

**Processed messages stay in the mailbox by default.** Each folder can instead move or delete messages once an order is processed or a draft accepted or rejected. Delete uses the mail server’s trash when available; without a trash folder it can permanently delete the message. The change applies only going forward. Uploaded files and unreadable messages are untouched. A failed mailbox action is retried up to five times and doesn’t undo the result in Kontor.

## Agent access

An MCP client can search your data, propose entries for the inbox, or link existing records. See [MCP setup and token scopes](mcp.md) for client examples and the difference between MCP and REST permissions.
