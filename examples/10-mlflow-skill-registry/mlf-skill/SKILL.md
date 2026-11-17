---
name: mlf-skill
description: Wraps MLflow model registry operations for common agent tasks
---

# MLF Skill

Wraps MLflow model registry operations for common agent tasks: register a new model version from a run, promote a version to a stage or alias, and print a short registry summary. Delegates identifier formatting and URI normalization to the bundled mlf-helper skill, and always shows the tracking URI in effect before running a mutation.
