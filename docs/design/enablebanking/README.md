# Enable Banking import: CSV versus API data

Kontor imports bank transactions from CSV files (DKB, comdirect) and, optionally, through the Enable Banking API (`backend/internal/enablebanking/`, `backend/internal/modules/ledger/bank_*.go`). This note compares what the two sources deliver, why their fingerprints cannot match, and how the API import avoids duplicating CSV history.

The fixtures next to this file are **synthetic**. They follow the public Enable Banking `Transaction` schema; no real API call and no real account data was used. `dkb-transactions.json` and `comdirect-transactions.json` hold two groups each:

- `csvEquivalent`: the rows of `docs/design/csv/DKB-csv-example.csv` and `comdirect-csv-example.csv` as an API would return them. The tests parse the CSV files with the real providers and sync these rows.
- `other`: a debit with remittance lines, a pending (`PDNG`) row, two identical same-day payments without `entry_reference`, a non-EUR row and a booked row that only has `value_date`.

Real field content (names, purposes, dates) has to be verified by the operator against their own banks (see "Verified by the user").

## Field mapping

| Kontor field (`ParsedRow`) | DKB CSV | comdirect CSV | Enable Banking API |
|---|---|---|---|
| Booking date | `Buchungsdatum` | `Buchungstag` | `booking_date`, falling back to `value_date`, then `transaction_date` |
| Value date | `Wertstellung` | `Wertstellung (Valuta)` | `value_date` |
| Amount | signed German decimal (`Betrag (€)`) | signed German decimal (`Umsatz in EUR`) | **unsigned** decimal string `transaction_amount.amount`; the sign comes from `credit_debit_indicator` (`DBIT` negative, `CRDT` positive) |
| Counterparty name | outgoing: `Zahlungsempfänger*in`; incoming: also the payee column, which holds the **user's own name** | parsed out of `Buchungstext` (`Empfänger:` / `Auftraggeber:`) | the **other** side: `creditor` for `DBIT`, `debtor` for `CRDT` |
| Counterparty IBAN | `IBAN` column | inside `Buchungstext` (not extracted) | `creditor_account.iban` / `debtor_account.iban` of the other side |
| Purpose | `Verwendungszweck` | `Buchungstext:` part | `remittance_information[]`, trimmed and joined with a space |
| Bank reference | `Kundenreferenz` | `Ref. …` suffix of `Buchungstext` | `entry_reference` (optional) |
| Transaction type | `Umsatztyp` | `Vorgang` | `bank_transaction_code.description` |
| Status | `Status` column (ignored; pending rows are imported today) | rows with `offen` are skipped | `status`: only `BOOK` is imported; `PDNG`, `HOLD`, `SCHD`, `CNCL`, `RJCT`, `OTHR` are counted and skipped |
| Currency | EUR only | EUR only | any; non-EUR rows are skipped and counted, never converted |

Differences that matter:

- **Counterparty side.** The DKB CSV names the user for incoming rows; the API names the sender. The API data is more useful for keyword matching, but the text differs, so fingerprints cannot agree.
- **Purpose text.** comdirect packs the card details and the reference into one text field; the API splits remittance lines. The joined text will not be byte-identical to the CSV purpose.
- **Booking versus value date.** Card payments often carry a booking date a day away from the CSV date. The fixture's comdirect card payment is booked on 2026-04-01 in the API and 2026-04-02 in the CSV on purpose.
- **Pending rows.** Skipped through the `BOOK` filter. A pending row that later books arrives with a new status and is imported then.
- **`entry_reference` is optional** and `transaction_id` is not stable across calls, so Kontor never uses `transaction_id`.

## Fingerprints and the cross-source rule

The CSV fingerprint (`Fingerprint`) hashes account, booking date, amount, counterparty, purpose and bank reference as exact strings. Because the API text differs, an API row can never produce a CSV fingerprint, and changing the CSV fingerprint would invalidate existing data, so it stays untouched.

API rows get their own namespace, `eb1:` + hex SHA-256 (hex-only CSV fingerprints can never collide with it). The hashed input is an unambiguous encoding: every field is prefixed with its byte length (`encodeFields`), so a `|` or digits inside a counterparty, purpose or reference can never shift the boundary between two fields and make two different payments collide:

- with `entry_reference`: account + reference only, so a bank that edits purpose or counterparty later does not create a duplicate (one reference carrying different contents gets a suffix; an exact repeat stays a duplicate);
- without: account + date + amount + counterparty + purpose + occurrence index among identical rows of the fetch, so two identical same-day card payments both import.

To avoid importing history that a CSV already holds, `CommitLedgerSync` runs the following inside the write transaction, in this order:

1. A row whose `eb1:` fingerprint is known, or that was absorbed earlier (`idx/xdup/{apiFingerprint}`), is a duplicate.
2. The remaining rows are matched against **free non-API transactions of the same account** with equal signed amount and currency and a booking date at most `bankCrossSourceDays` (2) days apart. Rounds go by distance (exact date, then one day, then two), and each CSV transaction absorbs at most one API row, so k API rows against m < k CSV rows import k − m.
3. A match is not stored as a transaction. It is recorded as `idx/xdup/{apiFp}` → CSV transaction ID and `idx/xabs/{csvTxnId}` → API fingerprint. Both keys make the decision stable: a later run with a shifted window recognizes the absorbed API row by fingerprint, and an already absorbed CSV row is not free for another API row.
4. Everything else is inserted.

The tests cover this in `bank_fingerprint_test.go` (the D1 test: for both banks the CSV and API fingerprints differ, and syncing the API fixture after the CSV import stores nothing and reports every row as a cross-source duplicate) and `store_bank_sync_test.go` (distance 0 to 3, sign, account and currency mismatches, count awareness, exact-date-first, reruns, shifted windows).

### Known limits

- **False positives.** A genuinely different payment with the same signed amount as an unmatched CSV transaction within two days, and no API counterpart of its own, is absorbed. Exact-date-first rounds, count awareness and the persisted absorption reduce this; `bankCrossSourceDays` is a named constant to tune after real data.
- **CSV after API.** A CSV imported over API-synced history is not deduplicated against it (the CSV fingerprint never matches an `eb1:` row). Import the CSV backfill first, then connect the bank.
- **Absorptions are not exported.** `idx/xdup/` and `idx/xabs/` are not part of the export. After a restore the rule is recomputed by the same matching on the next sync.
- **Booking-date fallback.** For a `BOOK` row without `booking_date` the value date (or transaction date) is used, which can shift a booking by a day. It does not affect the `entry_reference` fingerprint.

## What the sync fetches

The first sync of an account without transactions uses `strategy=longest` (as much history as the bank allows; consent asks for `transactions_extended_history`). Afterwards the window starts at the newest stored booking date **not after today** minus 14 days (a booked row with a bad future date must not become the anchor, or the gap before it would never be fetched again; an account whose rows are all in the future falls back to `longest`), so late postings are picked up and the overlap deduplicates by fingerprint. All pages are fetched before anything is stored, and the commit goes oldest first in chunks, so an interrupted run cannot skip older rows.

## Verified by tests (fixtures and a fake server)

Conversion, amounts, fingerprints, the window function, cross-source matching, the sync flows and their error handling, the consent flow, the public callback, scopes and the absence of secrets in logs and responses.

## Verified only by the user (manual checklist)

The real API was never called. Before relying on the import, check against your Enable Banking app and banks:

1. Registering the app, uploading the certificate and registering exactly `PUBLIC_URL/api/v1/ledger/bank/callback` as redirect URL; linking your own accounts (restricted mode) and consenting to DKB and comdirect with the strong customer authentication they ask for.
2. The real ASPSP names, so the `verified` marker (name contains `dkb` or `comdirect`) fits.
3. Whether `required_psu_headers` of the ASPSP blocks unattended access (Kontor sends no `Psu-*` headers), and the real per-day call limit for unattended access (the default 24 h interval and the "Sync now" button are the only levers).
4. Whether Enable Banking accepts the `transactions: true` access flag; if not, the flag has to be dropped.
5. The maximum consent validity per bank and that the stored expiry date matches the app.
6. Data quality against your CSV exports: counterparty and purpose texts, booking versus value dates, `entry_reference` presence, and how many rows the first `longest` backfill returns.
7. After a CSV backfill: connect the bank, run a sync and confirm that existing rows are absorbed (imported count 0 for overlapping days) and that nothing is duplicated; tune `bankCrossSourceDays` if the real date gaps are larger.
8. The expiry mails at 14 and 3 days (needs SMTP) and the re-authorization flow.
