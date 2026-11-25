# Test-runner image for the striatum examples.

# uv 0.13.8 (digest-pinned; Dependabot's docker ecosystem keeps it current)
FROM ghcr.io/astral-sh/uv@sha256:a3ea1357342c1a4bf9a0c53a3deb1b55379aefa037338d8113d5771efa01774f AS uv

# golang 1.27, matching the version declared in go.mod
FROM golang@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf

COPY --from=uv /uv /uvx /usr/local/bin/

WORKDIR /opt/striatum-examples

# uv.lock is the single source of truth for Python versions; Dependabot's
# uv ecosystem keeps it up to date. Sync is frozen so the image always
# matches the committed lock exactly.
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen

# Put the synced venv (mlflow and friends) on PATH for the test run.
ENV PATH="/opt/striatum-examples/.venv/bin:$PATH"

# Run as a non-root user: the tests write only to temp dirs and an isolated
# HOME, so they never need elevated privileges, and a non-root default keeps
# mounted volumes (repo checkout, and especially the Docker socket, if you
# pass one so the tests can start their registry) safer.
RUN useradd --create-home --uid 10001 tester
USER tester
ENV HOME=/home/tester
