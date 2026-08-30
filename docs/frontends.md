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
  ui/             # cloud-presets domain UI: card + search match-highlight (@cloud-presets/ui)
  api-client/     # generated OpenAPI 3.1 client (@cloud-presets/api-client)
.coldcrabby-ui/   # vendored @coldcrabby/ui source — git-ignored, always-latest main
openapi/
  openapi.json    # the spec the client is generated from
tools/
  stub-api/       # dependency-free stand-in for the Go API
  sync-ui.sh      # clone/update the vendored @coldcrabby/ui checkout
```

Both apps follow the slicer UI's conventions: standalone components, signals,
`ChangeDetectionStrategy.OnPush`, SCSS, Vitest, and pnpm. The shared design
language and app-agnostic primitives (buttons, inputs, selects, …) come from
[`@coldcrabby/ui`](https://github.com/ColdCrabby/ui); only the pieces that are
specific to _this_ product — the catalog card and search match-highlight — live
in `packages/ui`. Both are consumed as TypeScript source through path mappings
(`@coldcrabby/ui`, `@cloud-presets/ui`, `@cloud-presets/api-client`), so a change
to a primitive is picked up by both apps without a separate build step.

## The shared UI design language (`@coldcrabby/ui`)

The SCSS design language (graphite + molten-amber accent, `html.dark` mode) and
the presentational Angular primitives are maintained in a separate repo,
[`ColdCrabby/ui`](https://github.com/ColdCrabby/ui), and shared across every Cold
Crabby frontend. There is **no published package** — it is consumed as _source_
from an always-latest, git-ignored checkout of `main` at `.coldcrabby-ui/`.

`tools/sync-ui.sh` (exposed as `pnpm sync:ui`) clones or fast-forwards that
checkout. It is a real working clone, so a local edit to the shared UI can be
committed there and pushed straight to `ColdCrabby/ui`. It runs automatically on
`postinstall` and — via `lefthook.yml` — on every `git pull`/branch switch
(`post-merge`, `post-checkout`), so the shared UI never goes stale. On a brand
new clone, run `pnpm sync:ui` before `pnpm install` (or just `pnpm install`,
which clones it, then re-run to link).

Each app wires it in three places:

1. **tsconfig `paths`** → the barrel (`@coldcrabby/ui` →
   `../../.coldcrabby-ui/src/public-api.ts`). The same block pins the shared
   runtime peers (`@angular/*`, `rxjs`, `ngx-markdown`, …) to the app's own copy
   so the app and the source-consumed UI never bundle two Angulars.
2. **Sass `includePaths`** (in `angular.json`) → `../../.coldcrabby-ui/src/styles`,
   pulled in once from `src/styles.scss` with `@use 'index'`.
3. **Runtime peers** in `app.config.ts` — `provideHttpClient()` (the `Icon`
   component fetches its SVGs) and `provideMarkdown()` (block tooltips) — plus
   the iconoir SVGs served under `/assets/icons` (asset globs in `angular.json`).

```ts
import { Button } from '@coldcrabby/ui';
```

```html
<button nexusButton variant="primary">Search</button>
```


## Commands

Run from the repository root:

| Command                   | Effect                                                        |
| ------------------------- | ------------------------------------------------------------ |
| `pnpm install`            | Install all workspace dependencies                           |
| `pnpm dev`                | **One unified dev origin at `http://localhost:5200`** (below) |
| `pnpm sync:ui`            | Clone/update the vendored `@coldcrabby/ui` source            |
| `pnpm gen:client`         | Regenerate the API client from `openapi/openapi.json`        |
| `pnpm build`              | Build both apps                                              |
| `pnpm test`               | Run every package's Vitest suite                             |

## Unified local dev — one URL, like production

`pnpm dev` (also `make dev`) stands up the whole stack behind a **single origin
that mirrors the deployment**: the Go server at `http://localhost:5200` serves
the public app at `/`, the vendor-admin app at `/vendor/`, and the API at `/v1`
(proxied to the stub API for sample data). Saving a file rebuilds that app and
the page reloads.

```
pnpm dev            # → http://localhost:5200
```

Because both apps are one origin, they cross-link with plain same-origin URLs —
the public header's **Vendor login** goes to `/vendor/`, the vendor header's
**Back to catalog** goes to `/` (configurable via each app's
`environment.{vendorUrl,publicUrl}`). Orchestration lives in
[tools/dev.mjs](../tools/dev.mjs); the Go server's dev behaviour is env-driven
(`PUBLIC_DIR`/`VENDOR_DIR`, `STUB_API_URL`, and the `*_DEV_URL` reverse-proxy
hooks) — see [cmd/server/main.go](../cmd/server/main.go).

`pnpm dev` serves each app's **built** output rather than its Angular dev server:
the apps and the source-consumed `@coldcrabby/ui` only share one Angular instance
through the esbuild build, so serving `dist` avoids the dev-server's
duplicate-Angular pitfall (`NG0203`). The frontend uses a **hoisted-free but
member-vendored** layout — `.coldcrabby-ui` is a pnpm workspace member and the
shared runtime packages are pinned to each app's copy via tsconfig `paths` — so
every tree bundles a single `@angular/core`.

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
