# Development

Install [Go](https://go.dev/dl/) (version in [go.mod](../backend/go.mod)), [Bun](https://bun.sh) (exact version in [package.json](../frontend/package.json)), [Task](https://taskfile.dev), and [Air](https://github.com/air-verse/air) for Go live reload. For linting, install the [golangci-lint](https://golangci-lint.run) version in [.tool-versions](../.tool-versions).

From the repository root:

```sh
(cd frontend && bun install --frozen-lockfile)
task dev
```

Open <http://localhost:5173>. Vite proxies API requests to the Go server on port 8080; the development task supplies a development-only signing key. Run `task ci` for the backend tests and lint plus the frontend lint, tests, and production build.

## Where to look in the code

- [Backend modules](../backend/internal/modules/): each module owns its HTTP routes, data model, storage, migrations, and export format. The [server](../backend/internal/server/) wires the modules together.
- [Frontend modules](../frontend/src/modules/): the registry drives navigation and routing in the React SPA. See the [frontend guide](../frontend/README.md) for the stack and development commands.
- [Bank import design note](design/enablebanking/README.md): why CSV and API transactions have different fingerprints and how Kontor avoids counting a payment twice.

`task build` builds the multi-stage Docker image locally. For a direct `docker build`, pass both `--build-arg BUN_VERSION=<version>` and `--build-arg GO_VERSION=<version>` from the files above. `task up` and `task down` run Docker Compose using the published image in [compose.yml](../compose.yml). For tags, mirrors, and CI publication, see [Publishing a release](releases.md).
