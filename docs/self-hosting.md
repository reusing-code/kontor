# Self-hosting Kontor

The [quick start](../README.md#run-locally) runs one container on port 8080. The [Compose file](../compose.yml) pulls the published image and mounts a named volume at `/app/data`. Keep the `.env` file with the signing key for subsequent starts. `docker compose down` stops the app and retains the data volume.

## Configuration

The backend reads environment variables; [backend/.env.example](../backend/.env.example) lists the settings and defaults. With Compose, `.env` supplies values for substitution: pass additional variables to the container under `services.kontor.environment` in the Compose file.

| Variable | Default | Purpose |
| --- | --- | --- |
| `JWT_SECRET` | Required | Session signing key, at least 32 bytes. |
| `REGISTRATION_ENABLED` | Unset | Unset allows the first registration only; `true` leaves sign-up open, `false` closes it. |
| `AUTH_RATE_LIMIT` / `AUTH_RATE_WINDOW` | `10` / `1m` | Login and registration attempts per client IP; `0` disables the limit. |
| `TRUST_PROXY` | `false` | Read client IPs from `X-Real-IP` or `X-Forwarded-For`. Enable only behind a reverse proxy that sets those headers. |
| `ENCRYPTION_KEY` | Unset | 32-byte key (raw or Base64) for stored mail passwords, paperless tokens, and bank sessions. |
| `PUBLIC_URL` | Unset | External origin for paperless back-links and bank consent redirects. |

Kontor allows IMAP and paperless hosts on private networks, including loopback. Keep registration closed unless you trust users to make connections from the server to those hosts. Changing a password ends that user's other login sessions; personal access tokens stay valid.

## Exports and backups

In **Settings**, download a versioned JSON export for one module or the whole account. Import restores into empty, enabled modules; it replaces their categories and, on a full import, settings. Re-enter email passwords and reconnect bank accounts after a JSON restore.

For full database snapshots, set `BACKUP_DIR` to a persistent directory (mount it separately with Compose). `BACKUP_INTERVAL` defaults to `24h` and `BACKUP_KEEP` to `7`; an unset directory disables snapshots. Restore a snapshot with the Badger CLI (`badger restore`).

## Service limits and monitoring

`/healthz` reports liveness, `/readyz` checks the database, and `/metrics` exposes Prometheus metrics. Production logs are structured JSON.

JSON bodies and `/mcp` requests are limited to 1 MiB, bank CSVs, contract imports, and quick-add files to 10 MiB, and export imports and `.eml` uploads to 64 MiB. Oversized requests return HTTP 413. The HTTP server allows 10 seconds for headers, 5 minutes to read a request, and 2 minutes of idle time. SMTP connections time out after 10 seconds and each mail after 60 seconds. On shutdown, the server allows 10 seconds for HTTP shutdown and another 15 seconds for background work before closing the database.
