# Publishing a release

CI publishes the image to `forgejo.reusing.dev/tobi/kontor` and mirrors it to `ghcr.io/reusing-code/kontor`, which `compose.yml` uses. Tags: `sha-<commit>`, `main`, and per release `X.Y.Z`, `X.Y` and `latest`.

Releases are created on Forgejo, either in the UI or by pushing a `v*` tag (CI then creates the release, with the message of an annotated tag as its text). For every `v*` tag CI appends the commits since the previous tag, the image and a compare link to the Forgejo release text and creates the same release on [GitHub](https://github.com/reusing-code/kontor), where the push mirror only brings the tag (`ci/publish-release`). This needs the `GH_RELEASE_TOKEN` secret, a fine-grained GitHub token for `reusing-code/kontor` with *Contents: Read and write*. To publish a missed release by hand, run `FORGEJO_TOKEN=<token with write:repository> GH_TOKEN=$(gh auth token) GH_REPO=reusing-code/kontor FORGEJO_API=https://forgejo.reusing.dev/api/v1/repos/tobi/Kontor ci/publish-release vX.Y.Z` (`DRY_RUN=1` prints the writes instead and needs no Forgejo token).

Every push to `main` adds a `sha-*` tag on Forgejo. A package cleanup rule set in the Forgejo UI (user settings → Packages → Cleanup rules → Add cleanup rule) keeps that bounded while keeping release tags, `main` and `latest`:

- Type: Container, "Apply pattern to full package name" checked
- Keep the most recent: 10 versions per package
- Remove versions matching: `^kontor/sha-`

The remove pattern limits the rule to the `sha-*` tags of `kontor` (not `kontor-ci`); the preview in the rule editor shows what would be deleted.
