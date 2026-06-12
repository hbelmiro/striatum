// Package installer handles artifact installation for Cursor, Claude Code, and
// project-level directories (copy from cache to target). For Workflow artifacts,
// it also creates an entrypoint symlink so Claude Code can discover the workflow.
package installer
