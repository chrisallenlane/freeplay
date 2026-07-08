# Releasing

How to cut a Freeplay release.

## Conventions

- **Versioning:** [SemVer](https://semver.org/). Bump *major* for breaking
  changes to user-visible behavior or operator-facing config; *minor* for
  new features; *patch* for bug fixes, dependency bumps, and build/CI
  changes that don't change runtime behavior.
- **Tag format:** `vX.Y.Z`, annotated (`git tag -a`).
- **Release commit:** `chore: release vX.Y.Z` on `master`.
- **Distribution:** Docker images, published by CI on tag push. There is
  no GitHub release page, no package registry. The image *is* the release
  artifact.

## What CI does on a `v*` tag

Both pipelines watch `tags: ['v*']` and produce a Docker image:

- **`.gitea/workflows/build.yaml`** runs `make check`, then builds and
  pushes `:vX.Y.Z`, `:latest`, and `:sha-<short>` to the maintainer's
  internal registry (host configured via the `REGISTRY_HOST`,
  `REGISTRY_USERNAME`, and `REGISTRY_PASSWORD` Gitea Actions secrets).
  `:latest` is published by the tag build (tracking the newest release,
  not master HEAD); plain master pushes get only `:sha-<short>`.
- **`.github/workflows/docker.yml`** is the public-facing pipeline: it
  pushes `ghcr.io/chrisallenlane/freeplay:X.Y.Z`, `:X.Y`, and (on the
  tag that is the newest semver) `:latest` to GHCR.

Both inject the tag into the binary as the `Version` ldflag, so
`freeplay --version` reports the tag.

## Procedure

Run [`/release`](#using-release) for the guided flow, or follow these
steps manually:

1. **Verify readiness.** Working tree clean, `master` up to date with
   `origin/master`, all checks green:
   ```sh
   git checkout master
   git pull --ff-only
   make check
   ```

2. **Confirm `CHANGELOG.md` has an `[Unreleased]` section** describing
   what's in the release. If empty, the release isn't ready — populate it
   first.

3. **Relabel `[Unreleased]`** to `[X.Y.Z] - YYYY-MM-DD`:
   ```markdown
   ## [1.2.0] - 2026-06-15
   ```

4. **Commit and tag:**
   ```sh
   git add CHANGELOG.md
   git commit -m "chore: release vX.Y.Z"
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   ```

5. **Push commit, then tag:**
   ```sh
   git push origin master
   git push origin vX.Y.Z
   ```

   Pushing the tag is what triggers the Docker builds. The commit-only
   push runs `make check` but does not publish anything tagged.

6. **Watch CI.** Both Gitea and GitHub Actions runs should go green.
   If a build fails, the image won't publish — investigate, fix on
   `master`, and decide whether to re-tag (delete the failed tag locally
   and on origin, then re-tag a fixed commit) or cut `vX.Y.Z+1`.

7. **Verify the image.** Pull and smoke-test the published tag:
   ```sh
   docker pull ghcr.io/chrisallenlane/freeplay:X.Y.Z
   docker run --rm ghcr.io/chrisallenlane/freeplay:X.Y.Z freeplay --version
   ```

## Using `/release`

The `/release` Claude Code skill automates the procedure above with
safety prompts. It:

- Verifies repo state (clean tree, up-to-date with origin).
- Runs `/review-release` as preflight (tests, build, debug-artifact
  scan, version-consistency, license check).
- Proposes a semver bump from commits since the last tag.
- Pauses for confirmation at the local→remote boundary.
- Halts on first failure rather than guessing at rollback.

If preflight reports blockers, the skill refuses to proceed. There is no
override flag.

## Hotfix / re-tagging

If a release tag was pushed but the build failed (or shipped a
regression that needs an immediate fix on the same tag), prefer cutting
a new patch version over moving the tag. Moving a published tag breaks
downstream pulls and image caches.

To delete a bad tag *that no one has pulled yet*:

```sh
git tag -d vX.Y.Z
git push origin :refs/tags/vX.Y.Z
```

This is only safe immediately after pushing, before CI publishes or
anyone fetches. Otherwise: cut `vX.Y.Z+1`.
