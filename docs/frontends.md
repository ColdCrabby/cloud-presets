# Frontends

Two Angular 22 applications and their shared packages live in a pnpm workspace
alongside the Go API. A single OpenAPI document generates the client both apps
consume, so a schema change is one atomic commit rather than a cross-repo dance.

## Layout

```
apps/
  public/         # Angular: browse & search the catalog
  vendor-admin/   # Angular: authenticated vendor management
packages/
  ui/             # shared standalone UI primitives (@cloud-presets/ui)
  api-client/     # generated OpenAPI 3.1 client (@cloud-presets/api-client)
openapi/
  openapi.json    # the spec the client is generated from
tools/
  stub-api/       # dependency-free stand-in for the Go API
```

Both apps follow the slicer UI's conventions: standalone components, signals,
`ChangeDetectionStrategy.OnPush`, SCSS, Vitest, and pnpm. Shared packages are
consumed as TypeScript source through workspace path mappings
(`@cloud-presets/ui`, `@cloud-presets/api-client`), so a change to a primitive is
picked up by both apps without a separate build step.

## Commands

Run from the repository root:

| Command                   | Effect                                                |
| ------------------------- | ----------------------------------------------------- |
| `pnpm install`            | Install all workspace dependencies                    |
| `pnpm gen:client`         | Regenerate the API client from `openapi/openapi.json` |
| `pnpm stub-api`           | Start the stub API on `http://localhost:8787`         |
| `pnpm start:public`       | Serve the public app on `http://localhost:4200`       |
| `pnpm start:vendor-admin` | Serve the vendor-admin app on `http://localhost:4201` |
| `pnpm build`              | Build both apps                                       |
| `pnpm test`               | Run every package's Vitest suite                      |

Each app's dev server proxies `/v1` to the stub API (see `proxy.conf.json`), so
`pnpm stub-api` in one terminal and `pnpm start:public` in another gives a
working end-to-end app before the real API is available.

## Client generation

The client is generated with [`@hey-api/openapi-ts`](https://heyapi.dev), which
consumes **OpenAPI 3.1 natively** — the format Huma emits — so no 3.0 downgrade
step is needed in the build. Configuration lives in `openapi-ts.config.ts`.

The generated output under `packages/api-client/src/generated` is **committed**,
so a spec change shows up as a reviewable diff in the frontends rather than as an
invisible build-time difference. Do not edit generated files by hand; run
`pnpm gen:client` instead.

`openapi/openapi.json` is currently a **stub** derived from
[`docs/api-surface.md`](./api-surface.md). The real Go API only serves
`/v1/health` so far (the root `openapi.yaml`); once the remaining endpoints in
`docs/api-surface.md` are implemented, replace this file with that export and
rerun `pnpm gen:client`.
