# Example 10: MLflow Skill Registry

This example shows a realistic Skill that wraps MLflow model registry operations, and then walks it through the MLflow Skill Registry. The `mlf-skill` artifact depends on a standalone helper, `mlf-helper`, and the pair demonstrates why self-contained pushes matter when a second system fetches your artifacts.

The flow uses two registries that should not be confused with each other. The OCI registry (localhost:5000) stores the striatum images, and the MLflow Skill Registry stores references to those images. MLflow never resolves striatum dependencies itself, so the image it registers must already carry the full closure.

## Layout

| Directory | Kind | Role |
| --- | --- | --- |
| `mlf-helper/` | Skill | Standalone helper pushed as an image |
| `mlf-skill/` | Skill | Root skill with one OCI dependency on `mlf-helper` |

The committed `mlf-skill/artifact.json` declares the dependency:

```json
"dependencies": [
  {
    "source": "oci",
    "registry": "localhost:5000",
    "repository": "examples/mlf-helper",
    "tag": "1.0.0"
  }
]
```

## Push the helper

The dependency must exist in the OCI registry before anything can resolve it:

```bash
cd mlf-helper
striatum pack
striatum push localhost:5000/examples/mlf-helper:1.0.0
```

## Push a self-contained image

Push the root skill with `--self-contained` so the resulting image bundles `mlf-helper` inside it, and capture the digest:

```bash
cd ../mlf-skill
striatum push --self-contained --print-digest localhost:5000/examples/mlf-skill:1.0.0
```

The flag flattens the dependency closure into a single OCI image. Without it, the image would carry only the root skill plus dependency references, and any consumer that cannot run striatum's resolver would fetch an incomplete artifact. The printed digest gives you an immutable reference, `repo@sha256:...`, that no later push of the `1.0.0` tag can change.

## Register the Skill with MLflow

MLflow 3.18 lets the Skill Registry track striatum images directly. Point the CLI at a local tracking store and register the digest reference you just printed. Use a database backend such as SQLite: the filesystem backend (`file:./mlruns`) is in maintenance mode and mlflow 3.x rejects it for skill registration:

```bash
export MLFLOW_TRACKING_URI=sqlite:///./mlflow.db
export MLFLOW_DISABLE_AGENT_HINT=1

mlflow skills register oci --image localhost:5000/examples/mlf-skill@sha256:<digest> --name mlf-skill
```

`MLFLOW_DISABLE_AGENT_HINT=1` silences the agent hint banner the CLI prints on every invocation.

The self-contained image is what makes this work. When MLflow later fetches the image through its OCI fetcher, it gets the complete Skill, root plus helper, in one pull, with no need to understand striatum's dependency graph.

## Pull the Skill back through MLflow

Anyone with access to the same MLflow tracking store can materialize the registered Skill into a local directory:

```bash
mlflow skills pull skills:/mlf-skill -d ./mlf-skill-pulled
```

The `skills:/` scheme resolves through the registry and writes the pulled image's contents under `./mlf-skill-pulled`. Because the reference is digest-pinned, the bytes you get back are exactly the bytes that were registered.

The same digest also installs through striatum itself, which is useful for teams that mix both tools:

```bash
striatum install --target claude localhost:5000/examples/mlf-skill@sha256:<digest>
```

Whether the Skill arrives via `mlflow skills pull` or `striatum install`, the content is identical because both resolve to the same immutable image.

## Running the test

The integration test for this example pushes both artifacts, registers the Skill with MLflow, and pulls it back. It requires the `mlflow` CLI (version 3.18 or newer) on PATH; when the CLI is absent, the test skips rather than fails. To get the pinned version CI uses, run `uv sync --frozen --project ../docker` and put `examples/docker/.venv/bin` on PATH.
