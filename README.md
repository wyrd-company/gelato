# Gelato

Gelato is a self-hosted Git server forked from Charm Bracelet Soft Serve. It keeps Soft Serve's Git-over-SSH, Git daemon, HTTP, LFS, repository browsing, mirroring, and hook behavior, and adds:

- OpenBao-signed SSH certificate authentication
- principal-driven authorization
- NATS admin commands
- NATS repository operation events
- optional push-on-update back to imported repository remotes
- Go and TypeScript packages for strongly typed NATS messages

## Quick Start

```bash
docker compose up --build
```

The demo stack starts:

- Gelato on SSH `localhost:23231`, HTTP `localhost:23232`, stats `localhost:23233`, and Git daemon `localhost:9418`
- NATS on `localhost:4222`
- OpenBao dev mode on `localhost:8200`
- an OpenBao setup job that enables the SSH secrets engine and creates a `gelato` signing role

## Configuration

Gelato accepts `GELATO_` environment variables. The upstream `SOFT_SERVE_` variables are still accepted for compatibility.

Important settings:

```yaml
openbao:
  enabled: true
  public_key_url: "https://openbao.example/v1/ssh/public_key"
  allow_insecure_http: false
  poll_interval: "1m"
  request_timeout: "5s"

nats:
  enabled: true
  url: "nats://nats:4222"
  subject_prefix: "gelato"
  admin_queue: "gelato-admin"
  request_max_skew: "5m"

remote_push:
  enabled: true
  timeout: "5m"
```

When `openbao.enabled` is true, plain SSH public keys and keyboard-interactive login are rejected. SSH user certificates signed by the cached OpenBao CA are required.
The OpenBao public key URL must use HTTPS unless `allow_insecure_http` is explicitly enabled for a local demo or trusted private network.

## Certificate Principals

Gelato treats the OpenBao-signed SSH certificate as the identity token. The certificate `ValidPrincipals` field drives authorization.

Supported principals:

- `role:admin` or `admin`: full server and repository administration
- `access-level:read` or `access-level:read-only`: read access
- `access-level:write`, `access-level:rw`, or `access-level:read-write`: read/write access
- `access-level:admin` or `access-level:admin-access`: admin access
- `repo:<repository>:read`, `repo:<repository>:write`, or `repo:<repository>:admin`: repository-specific access
- `user:<name>`, `username:<name>`, or `email:<name>`: display/logging identity

Traditional user, public key, JWT, and token SSH management commands are hidden when OpenBao auth is enabled.

## NATS Admin

Gelato subscribes to:

```text
gelato.admin.repo.*
```

Supported action subjects:

```text
gelato.admin.repo.create
gelato.admin.repo.rename
gelato.admin.repo.branch
gelato.admin.repo.tag
```

Every request must be a MessagePack `AdminEnvelope` with the NATS header:

```text
Content-Type: application/vnd.msgpack
```

Envelope:

```json
{
  "payload": "<raw JSON bytes>",
  "signature": {
    "format": "ssh-ed25519",
    "blob": "<signature bytes>"
  },
  "certificate": "<OpenBao-signed SSH public certificate in authorized_keys format>"
}
```

Gelato verifies that:

1. the certificate is signed by the cached OpenBao CA
2. the JSON payload includes a unique `requestId`
3. the JSON payload timestamp is inside `nats.request_max_skew`
4. the `requestId` has not already been used inside the skew window
5. the signature verifies against the certified public key
6. the certificate principals include admin access

The signed JSON payload contains `requestId`, `action`, `timestamp`, and action-specific fields. The signature covers exactly the UTF-8 bytes of that JSON payload; the outer NATS envelope is MessagePack.

Create repository:

```json
{
  "requestId": "01JY5JH7P3J6B6G9S0M4J9FJ9V",
  "action": "repo.create",
  "timestamp": "2026-06-19T00:00:00Z",
  "repository": "platform/api",
  "projectName": "Platform API",
  "remote": "git@github.com:wyrd-company/platform-api.git",
  "mirror": true
}
```

When `remote` is present, create is functionally an import. `mirror` imports as a mirror.

Rename repository:

```json
{
  "requestId": "01JY5JHAP7CN7R71MZ21R2CH4A",
  "action": "repo.rename",
  "timestamp": "2026-06-19T00:00:00Z",
  "repository": "platform/api",
  "newName": "platform/service-api"
}
```

Branch operations:

```json
{
  "requestId": "01JY5JHC9VDB70AFRDVKTTJQ36",
  "action": "repo.branch",
  "timestamp": "2026-06-19T00:00:00Z",
  "repository": "platform/api",
  "operation": "create",
  "branch": "main",
  "fromRef": "HEAD"
}
```

`operation` may be `list`, `default`, `create`, `delete`, `remove`, or `rm`.

Tag operations:

```json
{
  "requestId": "01JY5JHF0YA0FHMD3NVTBWN2BG",
  "action": "repo.tag",
  "timestamp": "2026-06-19T00:00:00Z",
  "repository": "platform/api",
  "operation": "create",
  "tag": "1.2.3",
  "ref": "main"
}
```

`operation` may be `list`, `create`, `delete`, `remove`, or `rm`.

## NATS Events

Events are MessagePack `RepositoryEvent` payloads with:

```text
Content-Type: application/vnd.msgpack
```

Each event is published twice:

```text
gelato.events.repo.<repo-path-tokens>.<event-type>
gelato.events.type.<event-type>.<repo-path-tokens>
```

For repository `platform/api` and event `repository.added`:

```text
gelato.events.repo.platform.api.repository.added
gelato.events.type.repository.added.platform.api
```

This allows subscribers to follow a repository nest path:

```text
gelato.events.repo.platform.>
```

or a specific event type:

```text
gelato.events.type.repository.added.>
```

Published event types:

- `repository.added`
- `branch.created`
- `repository.renamed`
- `repository.tagged`
- `repository.remote.pushed`
- `repository.remote.pulled`

## Remote Push On Update

When `remote_push.enabled` is true, Gelato's update hook pushes changed refs back to the first configured Git remote.

- mirror repositories use `git push --mirror <remote>`
- non-mirror imported repositories push or delete the updated ref

Successful pushes publish `repository.remote.pushed`. Mirror pulls from the scheduled mirror job publish `repository.remote.pulled`.

## Message Packages

Go package:

```go
import "github.com/wyrd-company/gelato/pkg/messages"
```

TypeScript package:

```bash
npm install @wyrd-company/gelato-messages
```

The TypeScript package exports message types, MessagePack encode/decode helpers, media type constants, and NATS subject helper functions.

## Build And Test

```bash
go test ./...
npm ci --prefix packages/gelato-messages
npm test --prefix packages/gelato-messages
docker build -t gelato:local .
```

In this devcontainer, Go commands may need `GOSUMDB=off` if the shared `/go/pkg/sumdb` cache is not writable.

## Release

Push a semver tag on `main` without a `v` prefix:

```bash
git tag 1.2.3
git push origin 1.2.3
```

The CD workflow publishes:

- container image `ghcr.io/wyrd-company/gelato:<version>`
- `@wyrd-company/gelato-messages` to npmjs
- `@wyrd-company/gelato-messages` to GitHub Packages
