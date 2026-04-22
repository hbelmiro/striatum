# Plan: Pluggable registries (OCI + Git), `artifact.json`, and resolution

## Terminology

- **Registry** (Striatum sense): a **pluggable artifact registry** — the interface and concrete backends that resolve references, inspect manifests, and fetch artifact content. **This plan implements** **OCI** and **Git** only; other transports are **out of scope** here but must remain **addable without breaking changes** (see **Forward compatibility**).
- **Manifest file** is **`artifact.json`** (singular; repo convention). This plan does **not** rename the file.
- **Not the same as** the legacy optional `dependencies[].registry` field (OCI base URL only). The **breaking** manifest redesign **replaces** that shape with explicit, backend-specific fields or a canonical `ref` (see below). The word **registry** in prose still means the Striatum **backend abstraction**, not a JSON key.

## Goals

- Introduce a **pluggable registry** abstraction so Striatum is not hard-wired to OCI distribution endpoints only.
- Keep **OCI** as the first-class registry implementation (behavior compatible with today’s CLI where it still applies).
- Add a **Git** registry implementation for the **consume path**: resolve deps, **inspect**, **pull**, and **install** from Git. **Push remains OCI-only** (see below).
- **Rethink dependency management** and the **`artifact.json` schema** together (**breaking changes OK** vs `v1alpha1`): no requirement to accept the current `v1alpha1` dependency shape at runtime.
- **Design for forward compatibility:** **`v1alpha2`** and the registry/ref model should allow **future** backends (e.g. ZIP bundles, REST / vendor Skills APIs) to be added **without** a new **`apiVersion`** bump and **without** CLI breaking changes — only new **`source`** / **ref** schemes and new registry implementations.

## Scope (this plan)

| In scope                                                                                                                                          | Out of scope (may follow later)                                        |
|---------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------|
| OCI + Git registries; consume path; **`striatum.dev/v1alpha2`** manifest; locked CLI shape (`pull` / `inspect` / `skill install`, **`oci push`**) | **ZIP** archive transport, **REST** / vendor **Skills API** registries |

## Current state (baseline)

- **`pkg/oci`**: pack, push, pull, inspect, reference parsing (`SplitReference`, `oci:` layouts, `host/repo/name:tag`).
- **`pkg/artifact`**: `Dependency` is `{ name, version, registry? }` — implicitly OCI (`registry/name:version`).
- **`pkg/resolver`**: walks the tree; builds fetch refs as `depReg + "/" + d.Name + ":" + d.Version`; `ResolvedArtifact` carries `Registry` (OCI base) + name/version.
- **`internal/cli`**: `remoteFetcher`, `ensureArtifactsInCache`, `pull`, `install`, `validate --check-deps` all assume OCI targets (`oras.ReadOnlyTarget`) and `oci.Pull` / `oci.Inspect`.

This coupling is the main thing a **registry** layer in **pkg** should replace behind stable interfaces.

## Registry abstraction

### Responsibilities (per implementation)

Backends do **not** need identical surfaces; align commands with what each storage type is good at.

| Capability                              | OCI registry | Git registry                            |
|-----------------------------------------|--------------|-----------------------------------------|
| Parse user reference → internal locator | Yes          | Yes                                     |
| Read manifest (inspect)                 | Yes          | Yes (e.g. `artifact.json` at repo path) |
| Fetch artifact files into a directory   | Yes (`Pull`) | Yes (sparse/shallow clone or archive)   |
| Push to remote                          | Yes          | **No** (by design; see below)           |

### Design note: `push` stays OCI-only

**Why not `striatum push` to Git?**

- **Different contract:** OCI registry push is **content-addressed / tag-based artifact upload**. Git push is **mutable history** (branches, merges, force-push, PR flow). Striatum would either reimplement a tiny, opinionated VCS client or surprise users when behavior diverges from their normal Git workflow.
- **Risk and policy:** Automated pushes raise questions (which branch, FF-only vs force, tag overwrite, signed commits) that teams already solve with CI and `git`; baking defaults into Striatum is easy to get wrong.
- **Clear product story:** Striatum stays **OCI-native for publishing**; Git is a **source / distribution read** path (like consuming a tagged release as a repo).

**Publishing to Git anyway:** use **`striatum pack`** (or future export) into a directory, then **standard Git** (commit, tag, push) or CI. Document that flow in `README.md` / demo when the Git registry ships.

### Suggested package layout

- `pkg/registry` (or keep `pkg/oci` at the leaf and add siblings): small **interfaces** + shared types (locators, errors).
- `pkg/registry/oci` (or continue using `pkg/oci` as the OCI implementation behind the interface).
- `pkg/registry/git`: Git implementation.

Avoid bloating `internal/cli` with transport logic; CLI should select a **registry implementation** from the reference scheme and call its APIs.

### Reference schemes (CLI UX)

- **OCI (unchanged)**: `host/repo/name:tag`, `oci:/path:tag` (and existing Windows rules).
- **Git (new)**: agree on a single unambiguous prefix, e.g. `git:https://github.com/org/repo.git@v1.0.0` or `git:https://...#ref` — **finalize in implementation** with tests for edge cases (submodules, private repos, SSH URLs).

Document the chosen grammar in `README.md` once stable.

## CLI shape (locked in; breaking changes allowed)

Some verbs are **backend-specific** (e.g. OCI **push** / **pack-to-layout** semantics do not map to Git). **Chosen approach:** **polymorphic reads** (one set of commands; **`<ref>`** selects the registry backend) and **OCI-only publish** under a dedicated **`oci`** subcommand.

### Command × backend matrix

| Concern                         | OCI            | Git             | Notes                                                                   |
|---------------------------------|----------------|-----------------|-------------------------------------------------------------------------|
| Read manifest / metadata        | inspect        | inspect         | Same verb, dispatch on ref                                              |
| Fetch artifact (+ deps) to disk | pull           | pull            | Same verb                                                               |
| Install skill to agent dirs     | skill install  | skill install   | Same verb                                                               |
| Bundle project to local layout  | pack           | pack (optional) | Today = OCI Image Layout; Git may skip or add `pack --format tar` later |
| Upload / publish                | push           | **omit**        | Use `git` for VCS publish; Striatum stays out of Git push               |
| Local project lifecycle         | init, validate | init, validate  | Backend-agnostic                                                        |

### Commands

- **`striatum pull`**, **`striatum inspect`**, **`striatum skill install`** — one command each; **ref** carries the backend (`host/...`, `oci:…`, `git:…`). **Additional ref schemes later** must be **additive** (same commands; new schemes only).
- **`striatum push`** → **`striatum oci push`** (or equivalent such as `striatum registry push oci` if you prefer a longer path) so publish is explicitly OCI, not universal.
- **`striatum pack`** — document as **OCI Image Layout** output; optionally later `striatum pack --format oci|dir`.

Users learn that **registry publish** lives under **`oci`**, not as a generic top-level `push`.

### Documentation

Update **README / demo** with a small table: “Command → backends supported.”

### Forward compatibility (future ZIP, REST, etc. — not implemented now)

Later backends must **not** force another **`apiVersion`** break or **CLI** break if we shape **`v1alpha2`** and the **registry layer** correctly now.

**CLI:** Keep **one** polymorphic read surface (`pull`, `inspect`, `skill install`). Adding a backend = **new ref scheme(s)** + implementation, **not** new top-level verbs.

**Manifest (`v1alpha2`):** Prefer a dependency shape that **extends** cleanly:

- New `source` values must be **addable** without invalidating existing documents — **reject unknowns** at validation time until that backend ships, but **do not** bake in OCI-only field names that block new sources.
- Avoid hard-coding **only** OCI field names at the top level of each dependency; use **namespaced** or **`source`-discriminated** objects so ZIP/REST fields do not collide.

**Code:** **`DependencyFetcher`** / registry **registration** should be **open for extension** (new backend id → new parser + fetcher) without changing resolver’s graph-walking contract.

**Publish:** Remains **per-backend** (`oci push` today; hypothetical vendor commands later). That rule does not change when ZIP/REST appear.

*Illustrative future refs (not specified or implemented in this scope):* `zip:…`, `https+skills:…` / vendor-prefixed schemes — **design in a follow-up**, not here.

## `artifact.json` format (breaking rethink)

This section is the **contract** for **`striatum.dev/v1alpha2`**. Implementation must match it after the pattern (**A**, **B**, or **C**) is locked in tests.

**Constraint:** Breaking changes vs **`v1alpha1`** are acceptable **now**. Tools accept **`v1alpha2` only**; no migration from `v1alpha1`.

### What changes from `v1alpha1`

| Area                             | `v1alpha1` (today)                                                                                                   | `v1alpha2` (planned)                                                                                                                                             |
|----------------------------------|----------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **`apiVersion`**                 | `striatum.dev/v1alpha1`                                                                                              | `striatum.dev/v1alpha2`                                                                                                                                          |
| **`kind` / `metadata` / `spec`** | Same intent (see below)                                                                                              | Same **field names and roles** unless you explicitly rename in implementation                                                                                    |
| **`dependencies[]`**             | Each item: **`name`**, **`version`**, optional **`registry`** (OCI base only). Resolution = `registry/name:version`. | **No** `name`+`version`+`registry` triplet as the **only** story. Dependencies are **OCI-shaped**, **Git-shaped**, or **`ref`**, per chosen pattern — see below. |

### Top-level object (required keys)

| Field              | Type   | Required | Rules                                                                          |
|--------------------|--------|----------|--------------------------------------------------------------------------------|
| **`apiVersion`**   | string | yes      | Exactly **`striatum.dev/v1alpha2`** for this schema revision.                  |
| **`kind`**         | string | yes      | Same as today (e.g. **`Skill`**). Extensible list in code.                     |
| **`metadata`**     | object | yes      | See **Metadata** below.                                                        |
| **`spec`**         | object | yes      | See **Spec** below.                                                            |
| **`dependencies`** | array  | no       | Omit or `[]` if none. Each element is a `source` + payload object (Pattern B). |

### `metadata` (unchanged semantics)

| Field                                                       | Required | Meaning                                                                                                                                              |
|-------------------------------------------------------------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| **`name`**                                                  | yes      | Artifact id (e.g. skill name). Used for install paths, cache keys, dedupe with **`version`**.                                                        |
| **`version`**                                               | yes      | **Artifact release version** (semver or project string). **Not** a Git SHA, **not** an OCI digest; those belong on **dependencies** or in **`ref`**. |
| **`description`**, **`authors`**, **`license`**, **`tags`** | no       | Optional strings / arrays as today.                                                                                                                  |

### `spec` (unchanged semantics)

| Field            | Required | Rules                                                                                                      |
|------------------|----------|------------------------------------------------------------------------------------------------------------|
| **`entrypoint`** | yes      | Non-empty; must appear in **`files`**.                                                                     |
| **`files`**      | yes      | Non-empty array of project-relative paths; validate `..` / absolute paths rejected (same safety as today). |

### `dependencies[]` — **Pattern B locked** (tagged union: `source` + payload)

**Decision:** Pattern B — each dependency is an object with a `source` discriminator and backend-specific fields. In Go, each source maps to a concrete struct behind a shared `Dependency` interface.

Each dependency is an **object** with:

| Field        | Required | Description                                                                                                                                                                                   |
|--------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **`source`** | yes      | **`oci`** \| **`git`** today. **Unknown values:** reject at validate time until that backend ships (**forward compatibility** = add new enum values later **without** changing `apiVersion`). |

**If `source` === `oci`**

| Field            | Required | Meaning                                                                                                                |
|------------------|----------|------------------------------------------------------------------------------------------------------------------------|
| **`registry`**   | yes\*    | OCI registry **host** or **host:port** **without** trailing path (e.g. `localhost:5000`). **Not** the repository path. |
| **`repository`** | yes      | Repository path **without** tag (e.g. `skills/my-skill`).                                                              |
| **`tag`**        | yes      | Image/tag (e.g. `1.0.0`).                                                                                              |

\*CLI **`--registry` default** may fill **`registry`** when resolving; manifest may still require it for **`validate` without flags** — **decide in tests** (either require in file or allow omit when default provided).

**If `source` === `git`**

| Field      | Required | Meaning                                                                                |
|------------|----------|----------------------------------------------------------------------------------------|
| **`url`**  | yes      | Clone URL (`https://…`, `git@…`, etc.).                                                |
| **`ref`**  | yes      | Branch, tag, or commit SHA to resolve.                                                 |
| **`path`** | no       | Path **within** repo to directory containing **`artifact.json`** (default: repo root). |

**Internal mapping:** Implementation derives a **canonical locator** (and optionally a **`ref` string** for cache keys / `installed.yaml`) from this object.

## Example

```json
"dependencies": [
  {
    "source": "oci",
    "registry": "localhost:5000",
    "repository": "skills/base-skill",
    "tag": "1.0.0"
  },
  {
    "source": "git",
    "url": "https://github.com/org/lib-skill.git",
    "ref": "v1.2.0",
    "path": "packages/skill"
  }
]
```

### Validation checklist (`artifact.Validate` equivalent)

1. **`apiVersion`** === `striatum.dev/v1alpha2`.
2. **`kind`** supported.
3. **`metadata.name`**, **`metadata.version`**, **`spec.entrypoint`**, **`spec.files`** as today; entrypoint ∈ files; path safety on **`spec.files`**.
4. **`dependencies`:** each item valid per `source`; **reject unknown `source`** values.
5. **Transitive consistency (resolver / optional validate flag):** after fetch, child **`metadata.name`/`version`** may be checked against **expected** identity if the plan adds **`expectName`/`expectVersion`** later — **optional** for first slice.

### Naming hygiene (OCI)

- In pattern **B**, **`registry`** means **only** the OCI **registry endpoint** (host side). **`repository`** is the **name path**. Do **not** overload one string to mean `host/repo` glued together unless pattern **A** `ref` does so by design.

### Forward compatibility (manifest)

- **`v1alpha2`** documents that **new** `source` values (e.g. `zip`) may appear in a **later release**; validators gain new arms **without** bumping **`apiVersion`**, as long as existing **`oci`** / **`git`** objects remain valid.
- Pattern **A** stays compatible by **router** extension only.

### Tooling

- **`striatum init`** writes **`apiVersion`: `striatum.dev/v1alpha2`** and an empty **`dependencies`** (or minimal example matching locked pattern).
- **`striatum validate`**: enforce the checklist above; **`--check-deps`** uses the new resolution path.

---

## Resolution & locators (downstream of manifest)

### Problem (legacy)

Transitive resolution assumed every dependency was OCI-shaped:

`trim(ociRegistryBase) + "/" + name + ":" + version`

That does not generalize to Git (or to future backends) without structured locators.

### Direction

1. **Parse `dependencies[]`** into a **normalized internal form** (locator + optional expected identity) from the `source` + payload objects.

2. **Replace or generalize `ResolvedArtifact.Registry`** with an opaque **locator** the **registry implementation** understands, e.g.:
   - `OCILocator{ … }`, `GitLocator{ … }` **in this scope**.
   - The same **pattern** (backend id + opaque locator) should accommodate **future** backends **without** changing the resolver’s **algorithm**, only **new locator types** and **fetcher** registration.

   Resolver walks the graph using **`DependencyFetcher`** keyed by **backend id** + locator, **not** by concatenated OCI strings.

3. **CLI defaults:** **`--registry`** (or split flags) supplies the default **OCI** base when a dependency omits it; analogous **`--git-default`** / config only if the schema allows partial Git deps — specify in tests.

4. **Deduplication key:** Prefer **`metadata.name@metadata.version@canonicalRefOrLocatorFingerprint`** so the same semantic version on two backends does not collide incorrectly; exact rule in tests.

## Implementation phases (TDD)

Follow the attached TDD skill phases; track this checklist in the PR description or a sub-checklist here.

**TDD progress:**

- [ ] Phase A: failing tests in place
- [ ] Phase B: scenario loop complete (pre-implementation)
- [ ] Phase C: implementation green
- [ ] Phase D: scenario loop complete (post-implementation)
- [ ] Phase E: refactor done (or N/A with reason)
- [ ] Phase F: overlapping scenarios merged, removed, or justified
- [ ] Phase G: post-overlap scenario audit clear (F↔G loop settled)
- [ ] Phase H: review/fix loop clear (generic-review + Go skill)

### Suggested test order

1. **`pkg/registry` + types**: locator parsing, validation helpers (table-driven).
2. **`pkg/artifact`**: **new schema** validation tests (required fields per `source`, reject legacy `v1alpha1`).
3. **`pkg/resolver`**: tests with a **mock `DependencyFetcher`** keyed by locator / backend id. Cover:
   - OCI-only and Git-only trees.
   - Mixed graphs (root OCI, dep Git) if in scope for v1.
   - Cycle detection and deduplication with the **new** identity key rules.
4. **`pkg/registry/git`**: unit tests with **git local bare repos** or `httptest` / ephemeral dirs (no network in default `go test`; optional integration tag for real remotes).
5. **`internal/cli`**: reference routing, **`validate`** against new manifest, **`--check-deps`** with new dependency shape, pull/install cache; **publish** tests on **`oci push`** only.

### Phase H

Run full generic-review workflow (Go code review skill) before merge.

## CLI and installer touchpoints

- **`push`**: **OCI-only**; **`striatum oci push`** (see **CLI shape**) replaces generic top-level `push`.
- **`pull` / `inspect` / `skill install`**: parse reference → **registry implementation** → inspect/pull (OCI or Git); **shared polymorphic verbs** per locked CLI shape.
- **`ensureArtifactsInCache`**: today OCI-only; becomes **registry-aware** using the resolved locator list (not `oras.ReadOnlyTarget` for every node).
- **`installed.yaml` / `Registry` field**: today stores OCI base for reinstall. Extend model to store **registry backend id** (e.g. `oci`, `git`) **+ serialized locator** (or a stable canonical ref string per backend) so `--reinstall-all` can re-fetch Git-installed skills.

## Dependencies (Go modules)

- Git registry will need a Git implementation (e.g. `github.com/go-git/go-git/v5` or shelling out to `git`). Prefer **library** for testability unless you explicitly want the `git` binary — record the choice in the PR.

## Out of scope (unless you expand the plan)

- **`striatum push` to Git** — intentional; use `pack` + Git/CI (see design note above).
- **ZIP** / archive-based and **REST** / vendor **Skills API** registries — **future**; must remain **addable** per **Forward compatibility** without breaking **`v1alpha2`** or the **CLI** contract.
- Non-Git VCS.
- Authentication UX beyond env/standard Git config (document assumptions).

## Open decisions (resolve early with tests)

1. ~~**`dependencies[]` JSON pattern**~~ — **resolved: Pattern B** (tagged union, `source` + payload). Each source is a Go struct behind a `Dependency` interface.
2. Exact **`git:`** reference syntax and whether `path` defaults to repo root or a fixed subpath.
3. Whether **mixed-backend** (OCI + Git in one tree) dependency resolution is required in the first slice or deferred (e.g. Git-only root first).
4. **Cache directory layout** for Git artifacts (still `name@version` vs commit-based folders).
5. **Exact spelling** of OCI publish: `striatum oci push` vs `striatum registry push oci` (behavior identical; choose one for docs and Cobra).

**Future (when adding ZIP / REST):** HTTPS / generic URL **ref disambiguation** — require explicit scheme or profile so a URL is not mistaken for OCI; decide without breaking **`v1alpha2`** manifests already in the wild.

---

*Working plan for pluggable registries, breaking `artifact.json` evolution, and resolution; update as decisions land.*
