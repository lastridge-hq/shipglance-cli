# Releasing

A release is a tag. CI builds, checksums and publishes the archives and the
Homebrew cask; one step happens on the ShipGlance control plane.

1. **Fixtures current.** In the server repository, `make cli-fixtures
   CLI_FIXTURES_DIR=<this repo>/testdata/golden`; commit any change here with
   the server commit from `testdata/golden/SOURCE`. `go test ./...` passes.
2. **Tag.** `git tag vX.Y.Z && git push origin vX.Y.Z`. The release workflow
   runs `goreleaser release` (secrets: `GITHUB_TOKEN`, `HOMEBREW_TAP_TOKEN`
   with write access to `lastridge/homebrew-tap`).
3. **Verify the release.** The GitHub release has six archives and
   `checksums.txt`; `brew install lastridge/tap/shipglance` installs it;
   `shipglance version` prints `X.Y.Z`.
4. **Tell the server.** Add `X.Y.Z` to `SHIPGLANCE_CLI_RELEASES` on the
   control plane (server repository: `docs/cloud.md`). Until this step the
   server advertises the previous release as latest, so no one is told to
   upgrade. The first release also switches the version headers on.
5. **Raising the minimum** (`SHIPGLANCE_CLI_MINIMUM`) refuses every older CLI
   with exit 7. Do it only for a security fix or an API retirement, after
   `cloud_public_api_requests_total` shows who is still on old versions.

macOS builds are not signed or notarized. The Homebrew cask clears the
quarantine attribute; a binary downloaded by hand needs
`xattr -d com.apple.quarantine shipglance` once.
