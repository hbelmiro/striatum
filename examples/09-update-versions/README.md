# Example 09: Update versions

This example covers striatum's versioning story end to end. Manifests always pin exact tags, never ranges, so an artifact.json that asks for `1.0.0` resolves to that tag and nothing else. The `striatum update` command is the escape hatch: it upgrades an installed artifact to the highest semver tag available in the registry.

The `changelog-skill` artifact is the vehicle for the walkthrough. You will publish it as `1.0.0`, publish a second `1.1.0` tag, install the older version, and then update it in place.

## Layout

| Directory | Kind | Role |
| --- | --- | --- |
| `changelog-skill/` | Skill | Skill that drafts changelog entries from commit history |

## How versioning works

Every reference in a manifest is exact. A dependency block names one registry, one repository, and one tag, and an installed artifact records the tag it came from. There are no floating tags such as `latest` or ranges such as `^1.0.0` in the manifest schema, which keeps resolution deterministic: the same manifest always produces the same dependency closure.

The cost of exact pins is that new versions do not arrive on their own. `striatum update changelog-skill` closes that gap by listing the tags in the registry, picking the highest semver one, and upgrading the installed artifact to it. The upgrade is a reinstall of the new tag, so the install record and the files on disk stay consistent.

## Publish 1.0.0

Start a local registry and push the skill as `1.0.0`. Any registry works here because this example has no dependencies, so adapt the host and port below to yours:

```bash
cd changelog-skill
striatum pack
striatum push localhost:5000/examples/changelog-skill:1.0.0
```

## Publish 1.1.0

Bump the version in `changelog-skill/artifact.json`:

```json
"metadata": {
  "name": "changelog-skill",
  "version": "1.1.0"
}
```

Then push the same directory again under the new tag. Both tags now exist in the registry side by side:

```bash
striatum push localhost:5000/examples/changelog-skill:1.1.0
```

Because OCI registries treat tags as mutable pointers over immutable digests, publishing `1.1.0` leaves the `1.0.0` image untouched. Anyone still pinning `1.0.0` keeps resolving to the exact bytes that were pushed first.

## Install the older version

Install the `1.0.0` tag explicitly to recreate an outdated installation:

```bash
striatum install --target claude localhost:5000/examples/changelog-skill:1.0.0
```

At this point `~/.claude/skills/changelog-skill/` holds the `1.0.0` content, even though the registry offers a newer tag.

## Update in place

Run the update command to move the installation forward:

```bash
striatum update changelog-skill
```

The command queries the registry for the tags on `examples/changelog-skill`, selects `1.1.0` as the highest semver tag, and reinstalls it over the old version. Without an argument, `striatum update` checks every installed artifact and upgrades the outdated ones, and `striatum update --check` lists what is outdated without changing anything.

## Verify

Confirm that the installed version moved to `1.1.0`:

```bash
striatum list --installed --target claude
```

The listing shows `changelog-skill` at version `1.1.0`, proving the update landed.
