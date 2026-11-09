# Fork change: Responses compatibility layer

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

## Controls and rollback

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

## Validation

Validated locally with Go `1.26.10` on Windows/amd64:

```sh
go test ./relay/responsescompat
go test ./relay
go test ./router ./relay
go build -o .responsescompat-build-check/new-api.exe .
```

The final build check used a temporary placeholder for the ignored `web/dist`
embed directory and removed it afterward. It verifies the main Go binary links
the modified packages; the normal Docker build replaces that placeholder with
the Bun-built frontend. The Docker CLI was not available in this workspace, so
the complete image build must run in CI or on a Docker host before release.

A byte-level comparison against the source ZIP found no missing files and no
other source changes beyond the five modified and four added files listed
above.
