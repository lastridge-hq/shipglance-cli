# shipglance

The command line for [ShipGlance](https://shipglance.com): read your deploy
board from a terminal, a script or a coding agent. What is deployed where,
what failed, what is behind its branch.

> **Pre-release.** Not published yet; build from source to try it.

## Set up

1. On your board, open **Settings → API tokens** and create a token. Name it
   after where it will run ("GitHub Actions deploy", "Claude on my laptop").
2. Export it:

   ```sh
   export SHIPGLANCE_TOKEN=sgt_…
   ```

The token names its workspace, so there is nothing else to configure. Tokens
are read-only; revoke one in the same settings page and anything using it
stops working immediately.

## Commands

| Command | What it answers |
|---|---|
| `shipglance whoami` | Which workspace, plan and server the token reaches |
| `shipglance status [--repo owner/name] [--workflow deploy.yml]` | Every monitored board: run status, attention, what is deployed, drift |
| `shipglance drift [--repo …] [--workflow …]` | Per deploy stage: the deployed commit and whether the branch has moved past it |
| `shipglance version` | This CLI's version |

Drift is reported honestly: `not checked` when nothing was compared — never
"up to date" by default — and `unknown` with a reason when a comparison could
not finish.

## Output for scripts and agents

With `--json`, or whenever stdout is not a terminal, every run prints exactly
one line:

```json
{"outcome":"ok","notice":null,"data":{ … the /api/v1 response … }}
```

- `data` is the server's `/api/v1` response body, unchanged. A refusal carries
  the server's typed error (`{"schemaVersion":1,"error":{"kind":…}}`); a
  failure before any response is `{"kind":…,"message":…}`.
- `notice` is set when a newer CLI is available.
- `outcome` and the exit code are one fixed table:

| Exit | `outcome` | Meaning |
|---|---|---|
| 0 | `ok` | Done |
| 3 | `transient` | Temporary: the workspace is starting, a provider is down, rate-limited, or the network failed. Retry later |
| 4 | `auth` | No token, not a token, unknown or revoked token |
| 5 | `usage` | A bad flag or argument, or a workspace/endpoint that does not exist |
| 6 | `internal` | The CLI crashed, or the server sent something this CLI does not understand — upgrade it |
| 7 | `upgrade_required` | This CLI is older than the server supports — upgrade it |
| 9 | `plan_locked` | Your plan does not include this feature |

Codes 1, 2 and 8 are reserved for `shipglance wait` (deploy failed, timed
out, not deployable).

## What the CLI sends

Only the requests you ask for, to your workspace, with your token and this
`User-Agent`:

```
shipglance-cli/<version> (<os>/<arch>; cli)
```

That is the whole of it: no telemetry endpoint, no identifiers, nothing about
your machine beyond OS and architecture. The server uses the version to tell
an outdated CLI to upgrade.

On a terminal, a newer release is mentioned on stderr at most once a day
(`SHIPGLANCE_NO_UPDATE_NOTIFIER=1` turns that off). In `--json` output it is
the `notice` field instead.

macOS builds are signed with a Developer ID and notarized by Apple.

## Build from source

```sh
go build -o shipglance ./cmd/shipglance
```

Tests replay responses recorded from the real server (`testdata/golden`,
with the server commit in `testdata/golden/SOURCE`):

```sh
go test ./...
```

## License

Source-available under the [Functional Source License 1.1, ALv2 future
license](LICENSE.md) (FSL-1.1-ALv2): use, modify and redistribute it for
anything except a product that competes with ShipGlance. Each release becomes
Apache 2.0 two years after it ships. This is not an OSI-approved open-source
license.
