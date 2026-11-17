# Example 08: Digest pinning

This example shows how to pin an OCI dependency to an immutable content digest. The `pinned-skill` skill depends on `pinned-helper`; once the helper is pushed, its digest is copied into the root's dependency declaration.

## Layout

| Directory | Kind | Role |
| --- | --- | --- |
| `pinned-helper/` | Skill | Standalone helper pushed as an image |
| `pinned-skill/` | Skill | Root skill with a digest-pinned dependency |

## Why pin a digest

A tag such as `1.0.0` is mutable: it can be moved to a different image or deleted outright. A digest like `sha256:9f86d0818...` identifies one specific image content forever. When a dependency carries a digest, striatum resolves that exact content, so the tag can move or disappear without affecting resolution.

## Push the helper and capture its digest

Pack and push the helper with `--print-digest`, which prints one canonical reference line:

```bash
cd pinned-helper
striatum pack
striatum push --print-digest localhost:5000/examples/pinned-helper:1.0.0
```

The output looks like `localhost:5000/examples/pinned-helper@sha256:<64 hexadecimal characters>`. Copy the `sha256:...` portion.

## Pin it in the root manifest

Paste the printed digest into the `digest` field of the dependency in `pinned-skill/artifact.json`:

```json
"dependencies": [
  {
    "source": "oci",
    "registry": "localhost:5000",
    "repository": "examples/pinned-helper",
    "tag": "1.0.0",
    "digest": "sha256:<paste the printed digest here>"
  }
]
```

The committed manifest omits the `digest` field because the value depends on your local push; it must match the exact format `sha256:` followed by 64 lowercase hexadecimal characters, or be left out entirely. The `tag` can stay alongside the digest for human readability, but once the digest is present it wins during resolution.

## Pack, push, and install the root

```bash
cd pinned-skill
striatum pack
striatum push localhost:5000/examples/pinned-skill:1.0.0
striatum install localhost:5000/examples/pinned-skill:1.0.0
```

Install resolves the dependency by digest, so the exact helper content you pinned is what lands in `~/.claude/skills/`, regardless of what the `1.0.0` tag points to now or later.
