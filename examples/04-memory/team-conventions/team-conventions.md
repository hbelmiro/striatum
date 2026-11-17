---
name: team-conventions
description: Team coding and review conventions to apply when writing or reviewing code
---

# Team Conventions

Conventions the whole team follows. Apply them when writing new code and when reviewing pull requests.

## Code style

- Keep functions small and focused on one responsibility
- Prefer table-driven tests over repeated setup blocks
- Name booleans after the condition they represent, for example `hasDependencies`

## Reviews

- Reviewers respond within one business day
- Every finding includes a suggested fix or a concrete question
- Approve only when the change would be safe to revert
