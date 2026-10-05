# Releasing

A release is a tag. CI builds, checksums and publishes the archives and the
Homebrew cask; one step happens on the ShipGlance control plane.

1. **Fixtures current.** In the server repository, `make cli-fixtures
   CLI_FIXTURES_DIR=<this repo>/testdata/golden`; commit any change here with
   the server commit from `testdata/golden/SOURCE`. `go test ./...` passes.
2. **Tag.** `git tag vX.Y.Z && git push origin vX.Y.Z`. The release workflow
   runs `goreleaser release` in the `release` environment (secrets below).
3. **Verify the release.** The GitHub release has six archives and
   `checksums.txt`; `verify-macos` passed (signed, hardened, notarized);
   `brew install lastridge/tap/shipglance` installs it; `shipglance version`
   prints `X.Y.Z`.
4. **Tell the server.** Add `X.Y.Z` to `SHIPGLANCE_CLI_RELEASES` on the
   control plane (server repository: `docs/cloud.md`). Until this step the
   server advertises the previous release as latest, so no one is told to
   upgrade. The first release also switches the version headers on.
5. **Raising the minimum** (`SHIPGLANCE_CLI_MINIMUM`) refuses every older CLI
   with exit 7. Do it only for a security fix or an API retirement, after
   `cloud_public_api_requests_total` shows who is still on old versions.

## macOS signing and notarization

The release job signs the darwin binaries with the Developer ID Application
identity (`Armen Shimoon (6T85S58P8C)`) and notarizes them with an App Store
Connect API key — GoReleaser does both from Linux. `verify-macos` then
downloads what was published and requires, on a real Mac, the Developer ID
signature, the hardened runtime and Gatekeeper's `source=Notarized Developer
ID`. A bare CLI binary cannot be stapled; macOS checks the notarization online
the first time it runs, as it does for every notarized command-line tool.

The release refuses to start if any secret below is missing: a published
release is never unsigned.

### One-time setup

Repository → Settings → Environments → **release**: restrict it to tags
matching `v*`, then add these environment secrets (or with `gh`, as shown).

1. **The signing certificate** — the same Developer ID Application `.p12`
   the fleetifai release uses. Export it from Keychain Access (My
   Certificates → "Developer ID Application: Armen Shimoon (6T85S58P8C)" →
   Export, with a password):

   ```sh
   base64 -i developer-id.p12 | tr -d '\n' | gh secret set MACOS_SIGN_P12 --env release -R lastridge/shipglance-cli
   gh secret set MACOS_SIGN_PASSWORD --env release -R lastridge/shipglance-cli
   ```

2. **A notarization API key.** App Store Connect → Users and Access →
   Integrations → App Store Connect API → Team Keys → +, access
   *Developer*. Download `AuthKey_<KEYID>.p8` (offered once) and note the
   Issuer ID shown above the key list:

   ```sh
   gh secret set MACOS_NOTARY_ISSUER_ID --env release -R lastridge/shipglance-cli
   gh secret set MACOS_NOTARY_KEY_ID --env release -R lastridge/shipglance-cli
   base64 -i AuthKey_<KEYID>.p8 | tr -d '\n' | gh secret set MACOS_NOTARY_KEY --env release -R lastridge/shipglance-cli
   ```

3. **The Homebrew tap token** — a fine-grained token with *Contents: read and
   write* on `lastridge/homebrew-tap` only:

   ```sh
   gh secret set HOMEBREW_TAP_TOKEN --env release -R lastridge/shipglance-cli
   ```

Delete the exported `.p12` and the `.p8` afterwards; the secrets are the
copies that matter. To rehearse signing locally before a release, export the
same five values (with `HOMEBREW_TAP_TOKEN=unused`) and run
`goreleaser release --snapshot --clean --skip=publish`.
