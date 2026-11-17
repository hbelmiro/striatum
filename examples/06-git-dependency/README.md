# Example 06: Git dependency

This example shows a Skill that depends on another skill hosted in a Git repository. The `release-notes` skill declares its helper with a `git` dependency instead of an OCI one.

## Layout

| Directory | Kind | Role |
| --- | --- | --- |
| `release-notes/` | Skill | Root skill with one Git dependency |

## The dependency declaration

The committed `release-notes/artifact.json` declares the helper like this:

```json
"dependencies": [
  {
    "source": "git",
    "url": "https://github.com/example/release-notes-helper.git",
    "ref": "main"
  }
]
```

A Git dependency always carries a `url` and a `ref` (a branch, tag, or commit SHA). Optional fields such as `path`, `commit`, `files`, `name`, `kind`, and `entrypoint` can scope or shape the resolved artifact, for example when the repository root is not the artifact root.

The URL above is a placeholder used in this repository so the example stays self-contained. In a real project you would point it at a reachable Git repository containing a striatum artifact. The integration test for Git dependencies substitutes a local fixture repository for this URL, so the committed example never needs network access.

## Pack, push, and pull

Pack and push the root skill like any other artifact:

```bash
cd release-notes
striatum pack
striatum push localhost:5000/examples/release-notes:1.0.0
```

Pulling resolves the Git dependency too, cloning the repository at the declared ref and materializing the helper alongside the root:

```bash
striatum pull localhost:5000/examples/release-notes:1.0.0 -o ./out
```

The output directory will contain `release-notes/` plus the helper resolved from Git. The same resolution happens on `striatum install`, which places the whole closure under `~/.claude/skills/`.
