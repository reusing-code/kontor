# Bank import with Enable Banking

The bank import is off until you configure it; the UI shows nothing about it while `/api/v1/capabilities` reports `bankImport: false`.

## Privacy

Bank data passes through [Enable Banking](https://enablebanking.com), an account-information provider licensed under PSD2. For a personal installation, restricted mode limits access to the operator’s own accounts.

- **Sent to Enable Banking:** the selected bank, consent duration and scope (read access to balances and transactions with extended history), the redirect URL, and the bank’s account UIDs and requested date ranges. Kontor does not send your ledger entries, categories, or notes.
- **Stored by Kontor:** consent validity, account details (UID, IBAN, name, currency), imported transactions, and booked balances. The session ID is encrypted with `ENCRYPTION_KEY` and is never logged or returned by the API.
- **Included in JSON exports:** imported transactions and booked balances, but not consents, mappings, or sync state. Reconnect the bank after restoring a JSON export.

## Setup

1. Register an application in the [Enable Banking Control Panel](https://enablebanking.com/cp/applications):
   - **Environment: Production.** A sandbox application only sees Enable Banking's mock banks, and an application cannot be moved between the environments later. Both use the same API URL; the application ID decides the environment, so `ENABLE_BANKING_API_URL` stays unset.
   - **Key:** generate an RSA key and a self-signed certificate (`openssl genrsa -out private.key 4096`, `openssl req -new -x509 -days 365 -key private.key -out public.crt -subj "/CN=kontor"`) and upload the certificate, or let the control panel generate the key in the browser and export it. Kontor reads the private key as PEM (PKCS#1 or PKCS#8). Keep it secret.
   - **Allowed redirect URLs:** see step 2.
   - **Description and contact details:** production applications ask for a description (for example "Self-hosted personal finance app that imports my own bank transactions read-only for private, non-commercial bookkeeping."), a data-protection email and privacy and terms URLs; for a personal install your own details are fine.
2. Register the redirect URL **exactly** `PUBLIC_URL/api/v1/ledger/bank/callback` in the application (for example `https://kontor.example.com/api/v1/ledger/bank/callback`). Kontor strips a trailing slash from `PUBLIC_URL`, so the registered URL has no double slash. To develop against the same application, also register the local callback (see "Development" below).
3. Activate the application: a new production application is "Inactive". Use "Activate by linking accounts" in the control panel and authorize your own accounts at the bank; the application then runs in restricted mode and can only access the linked accounts. This only activates the application: Kontor still needs its own consent, given in step 5.
4. Set `ENABLE_BANKING_APP_ID`, `ENABLE_BANKING_KEY_PATH` (mount the key file read-only into the container, see [the Compose file](../compose.yml)), `PUBLIC_URL` and `ENCRYPTION_KEY`. Optional: `ENABLE_BANKING_SYNC_INTERVAL` (default `24h`; `0` disables only the automatic sync, expiry mails keep working), `ENABLE_BANKING_API_URL` (default `https://api.enablebanking.com`). Configure SMTP for the expiry mails.
5. Open Ledger → Bank connections, connect a bank, approve at the bank, then map each bank account to a ledger account (matched by IBAN automatically; or create the ledger account from the bank account).

Consents last up to 180 days. Kontor warns before expiry in the UI and, if SMTP is configured, sends email reminders 14 and 3 days before expiry. Re-authorizing keeps the account mappings.

**Two residual risks of the public callback.** The callback is public by design, so its `state` is not bound to the browser that started the consent: if someone gets you to finish a consent that was started from *their* Kontor session, your bank session would be attached to their user. Restricted mode and a single-owner install make this very unlikely, but do not open bank-consent links you did not start yourself. And `code` and `state` are part of the callback URL, so a reverse proxy's access log will contain them; both are single use and short lived (the state lives 15 minutes), but do not keep those access logs longer than needed.

Start with a CSV import for the history you already have and connect the bank afterwards; a CSV imported over API-synced history is not matched against it. How CSV and API data differ and how duplicates are avoided is described in [the CSV and API design note](design/enablebanking/README.md).

## Development

The consent redirects the browser to `PUBLIC_URL`, so for local development set `PUBLIC_URL=http://localhost:5173` (the Vite dev server proxies `/api` to the backend) and register that origin's callback URL in the Enable Banking application. Tests use an in-memory fake API (`internal/enablebanking/enablebankingtest`) and never contact Enable Banking.
