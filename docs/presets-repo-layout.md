# Presets Repository Layout

How [`ColdCrabby/presets`](https://github.com/ColdCrabby/presets) is organised,
and how vendor ownership is expressed as files rather than database rows.

The preset file format itself is specified in
[preset-schema.md](./preset-schema.md).

## Table of Contents

1. [Directory Layout](#directory-layout)
2. [Ownership Is Path-Based](#ownership-is-path-based)
3. [The Vendor Manifest](#the-vendor-manifest)
4. [Preset IDs and File Names](#preset-ids-and-file-names)
5. [Process Presets and the Shared Namespace](#process-presets-and-the-shared-namespace)
6. [CI Validation](#ci-validation)
7. [Branch Protection and Review](#branch-protection-and-review)

---

## Directory Layout

```
presets/
├── vendors/
│   ├── prusa/
│   │   ├── vendor.yaml
│   │   ├── printers/
│   │   │   ├── prusa-mk4-0.4.yaml
│   │   │   ├── prusa-mk4-0.6.yaml
│   │   │   └── prusa-mini-0.4.yaml
│   │   └── filaments/
│   │       └── prusament-pla-galaxy-black.yaml
│   │
│   └── bambu/
│       ├── vendor.yaml
│       ├── printers/
│       └── filaments/
│
├── processes/                  # community-maintained, vendor-neutral
│   ├── coldcrabby-standard-0.20.yaml
│   ├── coldcrabby-draft-0.30.yaml
│   └── coldcrabby-fine-0.12.yaml
│
└── .github/workflows/          # validation on every PR
```

One preset per file. The file is the unit of review, of ownership, and of
history — `git log` on a path is that preset's changelog.

Note that commits proposed through Vendor Admin are authored by the bot, so
`git blame` attributes them to the app rather than to a person. Human provenance
lives in the commit trailer and the pull request, not in the author field.

---

## Ownership Is Path-Based

**A preset's owner is determined by where the file lives, not by what it
contains.**

This matters because `vendor` is a free-text string in the slicer schema, and
process presets have no `vendor` field at all. If ownership were read from the
file body, any contributor could claim any vendor by typing its name. Path-based
ownership cannot be spoofed by editing a file — it requires write access to a
directory, which is exactly what GitHub's CODEOWNERS and the cloud's
authorization both key on.

The rule:

- Everything under `vendors/<slug>/` is owned by the vendor whose manifest
  declares `<slug>`.
- Everything under `processes/` is **project-owned** and has no vendor. It is not
  writable through the API at all.
- For printer and filament presets, the file's `vendor` field **must** match the
  owning vendor's display name or one of its declared brands. A value outside
  that set is a validation error, so the file body and its location can never
  disagree.

These are two distinct path invariants, and the validator applies them
separately. A preset under `vendors/` must resolve to a `vendor.yaml`; a preset
under `processes/` must **not** have one. A file outside both trees is an error.

---

## The Vendor Manifest

Each vendor directory contains a `vendor.yaml`. This file is the binding between
a GitHub directory, a public brand, and an authenticated Stytch Organization:

```yaml
slug: prusa
name: Prusa
website: https://www.prusa3d.com

# Additional brand names this vendor may use in a preset's `vendor` field.
# One company frequently ships hardware and filament under different brands.
brands:
  - Prusa
  - Prusament

# Stytch B2B Organization allowed to propose changes to this directory
# through the Vendor Admin app.
stytch_organization_id: organization-live-1a2b3c4d-...

# GitHub identities that may review and merge changes here.
# CODEOWNERS is generated from this list — see "Branch Protection and Review".
maintainers:
  - github: prusa-maintainer-bot
  - github: some-prusa-engineer

contact:
  email: presets@prusa3d.com
```

`brands` exists because the display name and the manufacturing brand often
differ: Prusa Research sells printers as **Prusa** and filament as
**Prusament**. Without it, either the filament preset would have to lie about its
brand or Prusament would need a separate namespace with separate ownership —
both worse than declaring the relationship once.

**This file is why the system needs no database.** The API resolves "may this
caller write to `vendors/prusa/`?" by comparing the organization claim in the
caller's session JWT against `stytch_organization_id` here. Identity lives in
Stytch; the *mapping* from identity to authority lives in Git.

The consequence worth stating plainly: **granting a vendor ownership is a pull
request.** It is reviewed, attributable, and has permanent history. The
equivalent database design — an `UPDATE` on a permissions table — has none of
those properties by default.

Note that `stytch_organization_id` is stored in a **public repository**, and is
therefore a public identifier rather than a secret. It grants nothing on its
own: authority requires a Stytch-signed JWT carrying that organization claim,
which only Stytch can issue.

`vendor.yaml` is validated like any preset file: `slug` must match the directory
name, be unique across the repository, `brands` entries must be unique across all
vendors, and `stytch_organization_id` must be well-formed.

---

## Preset IDs and File Names

**The file name must equal the preset `id` plus `.yaml`.** A file declaring
`id: prusa-mk4-0.4` lives at `prusa-mk4-0.4.yaml`. This makes the mapping from a
URL to a file mechanical, and makes duplicate IDs a merge conflict rather than a
subtle runtime collision.

ID rules:

- Lowercase, matching `^[a-z0-9]+(?:[-.][a-z0-9]+)*$`
- Unique across the **entire catalog**, not just within a type or a vendor
- Prefixed with the vendor slug for vendor-owned presets (`prusa-mk4-0.4`)
- **Stable forever.** IDs appear in URLs and in the `based_on` field of profiles
  users have already imported into their local libraries.

Renaming an ID is a breaking change: it invalidates links and orphans the lineage
of every local copy derived from it.

**Retiring a preset is a tombstone, not a deletion.** Removing the file outright
would free the ID for reuse, and a later preset claiming it would silently
inherit the old one's URLs and `based_on` references — pointing users at an
unrelated printer. Instead, retirement moves the entry to a Git-backed tombstone
that reserves the ID permanently and may name a successor. The API then answers
`410 Gone` for that ID rather than `404`, so clients can tell "withdrawn" from
"never existed".

Users who already imported the preset keep their local copy, because imports are
deep copies rather than live references. That is deliberate — a withdrawn preset
must never disappear out from under someone mid-project — but it also means a
withdrawal is only a *signal*, and acting on it is the slicer's choice.

A useful convention for printers is to encode the distinguishing hardware
variable in the ID, since the flat format means each variant is its own file:
`prusa-mk4-0.4`, `prusa-mk4-0.6`, `prusa-mk4-0.8`.

---

## Process Presets and the Shared Namespace

Process presets sit in a top-level `processes/` directory rather than under a
vendor, because quality profiles are **not vendor-specific** — "0.20 mm
standard" is a property of how you want to print, not of who made the machine.
The slicer's process schema reflects this by having no `vendor` field.

`processes/` is therefore a shared namespace, maintained by the project and open
to community contribution through ordinary pull requests. It has no
`vendor.yaml` and no Stytch organization mapped to it, which means **the Vendor
Admin app cannot write to it.** Changes arrive only as PRs reviewed by the
project maintainers.

That asymmetry is intentional. Vendor directories have a clear owner who is
accountable for correctness on their own hardware. A shared namespace has no such
owner, so it gets human review instead of delegated write access.

---

## CI Validation

Every pull request runs the same validation the cloud runs at ingest. Running it
in CI is what makes the vendor workflow tolerable: an author learns a file is
malformed on their PR, not hours later when someone notices the catalog went
stale.

### One validator, not two copies

The presets repository **does not vendor its own copy of the schemas.** CI
invokes the validator published by `cloud-presets` — a released binary or
container image that carries the pinned schemas inside it — referenced by an
**immutable digest committed to the presets repository**.

This is deliberate. Two vendored schema copies, one per repository, would drift
the moment one is bumped and the other is not, and the failure mode is
particularly nasty: CI passes, the PR merges, and *ingest* rejects the commit, so
the catalog silently freezes on an older revision.

Pinning by digest is what makes CI and ingest actually agree. Ingest is required
to validate with the digest the presets repository pins, so "CI ran a newer
validator than production" cannot happen. Merely having both track "latest" would
reintroduce exactly the drift this design removes — the two repositories deploy
on independent schedules.

Upgrading the validator is therefore an explicit, reviewable pull request against
the presets repository, and the full-catalog check in that PR proves the whole
catalog still validates under the new version *before* it reaches production.

### What CI checks

1. **Schema** — every changed preset validates against its type's draft 2020-12
   schema, with unknown fields rejected.
2. **Semantic ranges** — bounded quantities the schema leaves unbounded
   (fractions, temperatures, densities) are in plausible ranges.
3. **Naming** — file name equals `id` + `.yaml`; ID matches the required pattern.
4. **Uniqueness** — no duplicate IDs anywhere in the repository, including
   against retired IDs held by tombstones.
5. **Ownership** — presets under `vendors/` resolve to a valid `vendor.yaml` and
   their `vendor` field matches that vendor's name or a declared brand; presets
   under `processes/` have no owning manifest; nothing lives outside both trees.
6. **CODEOWNERS is current** — regenerated from the `maintainers` lists and
   compared against the committed file.
7. **Full-catalog build** — the entire repository is ingested exactly as the
   server would, because a set of individually valid files can still fail as a
   whole (a duplicate ID across two vendors, for instance).

Step 7 is the one that matters most. Per-file validation cannot catch
cross-file invariants, and those are precisely the failures that would otherwise
reach `main` and stall ingest.

---

## Branch Protection and Review

`main` is protected. Changes reach it only by pull request.

- **CODEOWNERS** maps `vendors/<slug>/` to that vendor's maintainers. It is
  **generated from the `maintainers` lists** in the manifests and verified in CI,
  so the two cannot disagree. `vendor.yaml` is the single authority; CODEOWNERS
  is a derived artifact that exists because GitHub only reads that file.
- **Required status checks** — the validation pipeline must pass before merge,
  which keeps the catalog buildable by construction.
- **Required review** — a vendor's own maintainers review changes to their
  directory; project maintainers review `processes/` and any `vendor.yaml`.

Changes to a `vendor.yaml` deserve particular scrutiny: that file grants API
write access, so a change to `stytch_organization_id` or `maintainers` is a
permission change and should be reviewed by project maintainers rather than the
vendor alone.

The bot's GitHub App permissions deliberately stop short of bypassing these
rules. It can push a branch and open a pull request; it cannot merge one.

Merging to `main` triggers re-ingest. Nothing else publishes: there is no way to
push a preset to the cloud that does not first land in Git.
