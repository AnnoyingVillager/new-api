# Fork-only changes

This file records the fork-only changes made around New API commit
`8faf1d45e8c101974dc7483324d3d91c8a462ffe`. Use it when reviewing a future
upstream sync. It is not part of New API's product documentation.

## Behavior added

- For an `input[]` item whose `type` is `function_call_output` or
  `custom_tool_call_output`, copy a non-empty `id` into `call_id` when
  `call_id` is missing or blank.
- In the default `move` mode, remove that source `id`, matching the existing
  local proxy workaround used for strict upstream APIs.
- Keep valid `call_id` values unchanged. Do not synthesize a random ID when
  neither field is usable.
- Restore `model` from the original client Responses request in non-streaming
  JSON (`.model`) and streaming events (`.response.model`).
- Apply the same request repair and response rewrite to the Responses HTTP,
  SSE, compact-request, and WebSocket frame paths.
- Leave unrelated JSON fields named `model`, binary WebSocket frames, error
  frames without a response model, billing, routing, and usage accounting
  unchanged.

## Daily email recipient limiter

- Counts successful quota reservations by recipient, so one SMTP or Resend
  request containing several recipients consumes the same number of units.
- Registration verification email stops when the current UTC day has reached
  the configurable registration threshold. Other mail continues until the
  separate hard threshold. Both defaults allow a request that reaches the
  threshold exactly and reject the next request.
- Defaults are 200 registration recipients and 290 total recipients. The
  default key expires at the next 00:00 UTC boundary, which is 08:00 in
  Asia/Shanghai. No reset job, database migration, or new container is needed.
- A Redis Lua script checks the current count and increments atomically. A
  request that would cross its effective threshold is rejected without
  incrementing the count. Redis is required while the limiter is enabled.
- The limiter is controlled by the namespaced option
  `fork.mail_rate_limit`, for example:
  `{"enabled":true,"registration_limit":200,"hard_limit":290}`.
  Administrators can edit it in **System Settings -> Operations -> Email rate
  limits**.
- Registration codes are stored only after mail delivery succeeds, so a quota
  rejection does not leave a code that the user could never receive.
- Password-reset responses keep the existing anti-enumeration behavior. All
  production email paths, including notifications and account-security mail,
  pass through the same backend transport gate. No frontend code decides
  whether an email may be sent.

## Changed files

- `relay/responsescompat/compat.go`: request repair, model lookup, and
  path-limited JSON/SSE response rewriting.
- `relay/responsescompat/middleware.go`: HTTP request preparation and a Gin
  response writer that buffers JSON and streams SSE.
- `relay/responsescompat/compat_test.go`: focused request, JSON, SSE, and
  middleware regression tests.
- `router/task-plugin-protocol-router.go`: installs the middleware on
  `POST /v1/responses`.
- `router/relay-router.go`: installs the same request/response middleware on
  `POST /v1/responses/compact`.
- `relay/responses_websocket.go`: repairs a normalized `response.create`
  request and rewrites text response frames before they reach the client.
- `relay/responses_websocket_test.go`: verifies text-frame rewriting and
  leaves binary frames opaque.
- `README*.md`: the same compatibility-layer notice in every top-level language, placed before the project description.
- `.github/workflows/build-fork-docker.yml`: builds the existing Dockerfile on
  GitHub Actions and publishes `linux/amd64` images to GHCR.
- `extensions/mailrate/`: isolated UTC-day configuration, Redis Lua
  reservation, recipient limits, and focused tests.
- `common/email.go` and `common/email_resend.go`: transport-level quota
  reservation shared by SMTP and Resend.
- `controller/misc.go`: classifies registration verification mail, returns 429
  or 503 for the relevant quota failures, and stores the code after delivery.
- `controller/misc_mail_rate_limit_test.go`: verifies the registration endpoint
  returns 429 before calling Resend when the lower threshold is exhausted.
- `model/option.go`: supplies and validates the namespaced default option.
- `web/src/features/system-settings/operations/mail-rate-limit-*.tsx|ts`: the
  Operations page, parsing, serialization, schema, and focused tests.
- The Operations registry, default values, types, static i18n key, and seven
  locale files.

No channel adapter, billing code, database schema, authentication code, or
Dockerfile change is required. The existing single-binary image includes the
new package automatically.

Before building, the fork workflow fetches upstream Git tags, finds the newest
upstream release tag reachable from the custom branch, and writes that tag to
`VERSION`. The displayed application version therefore tracks the upstream
release while remaining independent of fork-only commits.

The fork workflow publishes these GHCR tags from `codex/responses-compat`:

```text
ghcr.io/annoyingvillager/new-api:responses-compat
ghcr.io/annoyingvillager/new-api:branch-codex-responses-compat
ghcr.io/annoyingvillager/new-api:sha-<commit>
ghcr.io/annoyingvillager/new-api:latest
```

Pushes to `codex/responses-compat` publish the stable, branch, and commit tags.
Other `codex/**` branches publish only their branch and commit tags, so they
cannot replace the stable or `latest` images. A manual `workflow_dispatch`
defaults its `ref` input to `codex/responses-compat`. The fork's `main` remains
an upstream-sync baseline and is not an image build branch.

Fork-specific integration points are marked with
`FORK-RESPONSES-COMPAT`. This marker is the quickest way to locate the pieces
to review or remove after an upstream sync:

```sh
rg -n 'FORK-RESPONSES-COMPAT' router relay 'README*.md'
```

The layer sits after request authentication and rate limiting, before the
existing Responses handlers. Its only request mutation is the narrow `id` to
`call_id` fallback. Response rewriting happens after handling, so channel
selection, retry, billing, usage accounting, and logging continue to use the
existing upstream objects.

## Mail limiter controls and rollback

Use **System Settings -> Operations -> Email rate limits** to disable the
fork-only limiter or change its thresholds. The effective limits must satisfy
`0 < registration_limit < hard_limit <= 300`. The runtime Redis key is
`fork:mailrate:v1:YYYY-MM-DD` in UTC and expires at the next UTC midnight.

For an immediate rollback without an image change, open the Operations page
and turn off **Enable daily email limits**, then save. To remove the feature
after an upstream sync, search for `FORK-MAIL-RATE-LIMIT`, remove the isolated
`extensions/mailrate/` package and marked integration points, then drop the
frontend section and its locale keys. No database schema rollback is needed;
the persisted option is inert once the code is removed.

## Responses compatibility controls and rollback

The layer is enabled when `RESPONSES_COMPAT_ENABLED` is unset.

| Variable | Default | Meaning |
| --- | --- | --- |
| `RESPONSES_COMPAT_ENABLED` | enabled | Set to `false`, `0`, `off`, or `no` to disable every fork behavior immediately. |
| `RESPONSES_CALL_ID_MODE` | `move` | `fill` adds `call_id` and keeps `id`; `move` adds it and removes `id`; `off` leaves requests unchanged. |

For an immediate rollback, set `RESPONSES_COMPAT_ENABLED=false` and restart the
container. No database rollback or image rollback is required.

## Upstream-sync review checklist

1. Before rebasing, set `RESPONSES_COMPAT_ENABLED=false` if an immediate
   behavioral fallback is needed. A container restart is enough.
2. Compare current `router/task-plugin-protocol-router.go` and
   `router/relay-router.go` with this fork. Reapply only the two
   `responsescompat.Middleware()` entries if upstream still lacks equivalent
   behavior.
3. Compare `relay/responses_websocket.go`. The fork has three intentional
   calls: patch the normalized payload, rewrite `writeClient` frames, and
   rewrite the terminal frame in `runRequest`'s deferred write.
4. If upstream fixes response model echo or the Chat converter's `call_id`
   fallback, verify both HTTP/SSE and WebSocket before removing the package.
   Fixing only `to_oai_chat_req.go` does not cover native Responses passthrough.
5. Remove `relay/responsescompat/`, the marked route/WebSocket integrations,
   the WebSocket test, and the README subsection together. Keep this file
   until the corresponding upstream behavior is confirmed after the sync.
6. Search for `FORK-MAIL-RATE-LIMIT`. If upstream adds equivalent recipient
   accounting and UTC-day reset behavior, compare its registration-vs-other
   classification, multi-recipient counting, atomicity, and fail-closed Redis
   behavior before removing this module.
7. Remove the Mail Rate Limit Operations section and marked locale keys only
   after the backend integration is removed. A persisted
   `fork.mail_rate_limit` row is safe to leave; it is ignored without the code.

## Validation

Validated locally with Go `1.26.10` on Windows/amd64:

```sh
go test ./relay/responsescompat
go test ./relay
go test ./router ./relay
go test ./extensions/mailrate
go test ./common ./model
go test ./controller -run TestSendEmailVerificationReturns429AtRegistrationQuota
go test ./... -run '^$'
go build -o .responsescompat-build-check/new-api.exe .

cd web
bun check -b
bun run test
bun run build
```

The all-package compile check and the final Go build passed after the frontend
build populated the ignored `web/dist` embed directory. The focused Windows
Go tests passed. A full `go test ./controller` run reached unrelated existing
tests that share SQLite files concurrently and failed on Windows file locking;
the controller package compile check and this feature's endpoint test passed. It verifies the main Go binary links
the modified packages; the normal Docker build replaces that placeholder with
the Bun-built frontend. The Docker CLI was not available in this workspace, so
the complete image build must run in CI or on a Docker host before release.

A byte-level comparison against the source ZIP found no missing files and no
other source changes beyond the five modified and four added files listed
above.
