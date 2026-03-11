# Migration to v0.26.2 Stable Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebase the fork's daily-log + MCP features onto upstream v0.26.2 stable, set up CI/CD for Docker image builds, and produce a clean `v0.26.2-ds.20260311` release ready for production deployment.

**Architecture:** Cherry-pick 4 fork commits onto the v0.26.2 tag, resolve known conflicts, update version and CI configuration, then verify the build and migration path against a production DB copy.

**Tech Stack:** Go 1.25.7, SQLite, Docker (multi-arch), GitHub Actions, pnpm/Node 22

**Constraints:**
- **No auto-commit**: All git commits require user confirmation
- **No push**: All pushes require explicit user instruction
- **No modification to `resources/.memos/`**: Production DB is read-only; use copies for testing

**Spec:** `docs/superpowers/specs/2026-03-11-migration-to-0.26.2-stable-design.md`

---

## Chunk 1: Branch Reconstruction

### Task 1: Prepare workspace

**Files:**
- None modified (git operations only)

**Context:** Current state: branch `chore/daily-log-migration` at commit `952c6f43`, 2 stash entries, upstream remote already configured with `v0.26.2` tag fetched.

- [ ] **Step 1: Drop stale stash entries**

```bash
git stash list
# Expected:
# stash@{0}: WIP on chore/daily-log-migration: 952c6f43 ...
# stash@{1}: WIP on main: ebb0f583 ...
git stash drop stash@{1}
git stash drop stash@{0}
```

- [ ] **Step 2: Clean test artifacts**

```bash
rm -rf resources/.memos_test_backup/
```

- [ ] **Step 3: Verify clean working tree**

```bash
git status
# Expected: nothing to commit, working tree clean
# (except the new docs/ files from brainstorming which are untracked)
```

- [ ] **Step 4: Stage and commit the design spec and plan docs**

Wait for user confirmation, then:

```bash
git add docs/superpowers/
git commit -m "docs: add migration design spec and implementation plan"
```

---

### Task 2: Create new branch from v0.26.2 and cherry-pick fork commits

**Files:**
- Conflict: `server/router/api/v1/v1.go`
- Conflict: `web/src/router/index.tsx`
- All other fork files apply cleanly

**Context:** Four fork commits to cherry-pick:
1. `ebb0f583` — daily-log system (2 known conflicts)
2. `38a472ba` — update readme
3. `f27f85b7` — MCP ability
4. `952c6f43` — improve MCP

- [ ] **Step 1: Create branch from v0.26.2 tag**

```bash
git checkout -b main-stable v0.26.2
```

Expected: `HEAD is now at 71263736 chore: fix codeowners`

- [ ] **Step 2: Cherry-pick daily-log commit**

```bash
git cherry-pick ebb0f583
```

Expected: CONFLICT in 2 files:
- `server/router/api/v1/v1.go`
- `web/src/router/index.tsx`

- [ ] **Step 3: Resolve conflict in `server/router/api/v1/v1.go`**

The conflict is at the route registration block. v0.26.2 has neither SSE nor daily-log routes. The fork adds both. Resolution: accept the incoming (fork) block that adds SSE + daily-log routes before the catch-all handler.

Open the file, find the conflict markers (`<<<<<<<`), and keep **only** the fork's additions (the `=======` to `>>>>>>>` block):

```go
	// Register SSE endpoint with same CORS as rest of /api/v1.
	gwGroup.GET("/api/v1/sse", func(c *echo.Context) error {
		return handleSSE(c, s.SSEHub, auth.NewAuthenticator(s.Store, s.Secret))
	})

	// Register Daily Log REST API (custom HTTP routes alongside gRPC-Gateway).
	s.RegisterDailyLogRoutes(gwGroup)
```

These lines go **before** the existing `handler := echo.WrapHandler(gwMux)` line.

- [ ] **Step 4: Resolve conflict in `web/src/router/index.tsx`**

The conflict is in the lazy-import section. v0.27.0 introduced `lazyWithReload()` wrapper; v0.26.2 uses plain `lazy()`. Resolution: keep v0.26.2's existing lazy imports (the `HEAD` block), then add the `DailyLog` import using the same `lazy()` pattern:

```typescript
const DailyLog = lazy(() => import("@/pages/DailyLog"));
```

Add this line alongside the other `lazy()` imports (alphabetically).

Also check the route definition section of the same file — the `DailyLog` route entry should use the same JSX pattern as other routes in v0.26.2.

- [ ] **Step 5: Complete the cherry-pick**

```bash
git add server/router/api/v1/v1.go web/src/router/index.tsx
git cherry-pick --continue
```

If prompted for commit message, keep the original: `feat: add daily-log system`

- [ ] **Step 6: Cherry-pick readme commit**

```bash
git cherry-pick 38a472ba
```

Expected: clean apply, no conflicts.

- [ ] **Step 7: Cherry-pick MCP commit**

```bash
git cherry-pick f27f85b7
```

Expected: likely clean (adds new files under `server/router/mcp/`). If `go.mod` conflicts, accept both changes (keep v0.26.2 deps + add `mcp-go`).

- [ ] **Step 8: Cherry-pick MCP improvement commit**

```bash
git cherry-pick 952c6f43
```

Expected: clean apply (modifies files under `server/router/mcp/` which were just added).

- [ ] **Step 9: Verify cherry-pick result**

```bash
git log --oneline -6
# Expected: 4 fork commits on top of v0.26.2 history
```

---

### Task 3: Post-cherry-pick adjustments

**Files:**
- Modify: `internal/version/version.go:12` — change Version string
- Delete: `store/migration/sqlite/0.27/` (2 files)
- Delete: `store/migration/mysql/0.27/` (2 files)
- Delete: `store/migration/postgres/0.27/` (2 files)
- Verify: `store/migration/sqlite/LATEST.sql`

- [ ] **Step 1: Update version constant**

In `internal/version/version.go`, line 12, change:

```go
var Version = "0.27.0"
```

to:

```go
var Version = "0.26.2-ds.20260311"
```

Note: after cherry-pick onto v0.26.2, this line will read `"0.26.2"` (from the tag). Change it to the fork version. If it still reads `"0.27.0"` (carried by a cherry-picked commit), still change it.

- [ ] **Step 2: Remove unreleased 0.27 migration files**

```bash
rm -rf store/migration/sqlite/0.27/
rm -rf store/migration/mysql/0.27/
rm -rf store/migration/postgres/0.27/
```

Verify:

```bash
ls store/migration/sqlite/ | sort -V
# Expected: 0.10/ 0.11/ ... 0.26/ LATEST.sql
# NO 0.27/ directory
```

- [ ] **Step 3: Verify LATEST.sql has v0.26.2 schema**

```bash
grep -c "uid" store/migration/sqlite/LATEST.sql
```

Check: the `idp` table definition should NOT have a `uid` column. If it does, restore it from v0.26.2:

```bash
git checkout v0.26.2 -- store/migration/sqlite/LATEST.sql store/migration/mysql/LATEST.sql store/migration/postgres/LATEST.sql
```

- [ ] **Step 4: Run go mod tidy**

```bash
go mod tidy
```

Expected: may add `mcp-go` and its transitive dependencies if not already present. Should not remove anything critical.

- [ ] **Step 5: Verify build compiles**

```bash
go build ./cmd/memos
```

Expected: successful compilation. If errors, they likely relate to API differences between v0.26.2 and v0.27.0 that the cherry-picked code assumes. Fix case-by-case (most likely in `v1.go` SSE handler or store imports).

- [ ] **Step 6: Commit adjustments (wait for user confirmation)**

```bash
git add -A
git status  # review changes
git commit -m "chore: set version 0.26.2-ds.20260311 and remove unreleased 0.27 migrations"
```

---

### Task 4: Replace main branch and tag

**Files:**
- None (git branch/tag operations)

**Context:** At this point `main-stable` has the clean history. The old `main` and `chore/daily-log-migration` branches point to the 0.27.0-based history.

- [ ] **Step 1: Verify commit history is clean**

```bash
git log --oneline -7
# Expected (newest first):
# <hash> chore: set version 0.26.2-ds.20260311 and remove unreleased 0.27 migrations
# <hash> feat: improve mcp with mcp-builder skill
# <hash> feat(mcp): add /mcp ability
# <hash> chores(doc): update readme
# <hash> feat: add daily-log system
# <hash> ... (v0.26.2 upstream history)
```

- [ ] **Step 2: Delete old branches (wait for user confirmation)**

```bash
git branch -D main
git branch -D chore/daily-log-migration
```

- [ ] **Step 3: Rename current branch to main**

```bash
git branch -m main-stable main
```

- [ ] **Step 4: Create release tag**

```bash
git tag v0.26.2-ds.20260311
```

- [ ] **Step 5: Verify final state**

```bash
echo "=== branch ===" && git branch
echo "=== tag ===" && git tag -l 'v0.26.2-ds*'
echo "=== log ===" && git log --oneline -6
echo "=== status ===" && git status
```

Expected:
- Single branch: `* main`
- Tag: `v0.26.2-ds.20260311`
- Clean working tree

**DO NOT push.** User will push when ready:

```bash
# User runs these manually when ready:
git push origin main --force-with-lease
git push origin v0.26.2-ds.20260311
```

---

## Chunk 2: Verify Migration & Build

### Task 5: Test database migration against production DB copy

**Files:**
- Read only: `resources/.memos/memos_prod.db`
- Create (temp): `/tmp/memos_migration_test/`

- [ ] **Step 1: Copy production database to temp location**

```bash
mkdir -p /tmp/memos_migration_test
cp resources/.memos/memos_prod.db /tmp/memos_migration_test/memos_prod.db
```

- [ ] **Step 2: Record pre-migration state**

```bash
DB="/tmp/memos_migration_test/memos_prod.db"
echo "=== schema version ===" && sqlite3 "$DB" "SELECT value FROM system_setting WHERE name='BASIC';"
echo "=== tables ===" && sqlite3 "$DB" ".tables"
echo "=== record counts ===" && for t in user memo resource memo_relation reaction user_setting system_setting; do echo "$t: $(sqlite3 "$DB" "SELECT count(*) FROM $t;")"; done
```

Expected: schema 0.25.1, tables include `resource` (not `attachment`), `memo_organizer` present.

- [ ] **Step 3: Apply migration SQL manually (simulating what the migrator does)**

```bash
DB="/tmp/memos_migration_test/memos_prod.db"

# 0.26/00 - rename resource to attachment
sqlite3 "$DB" "
ALTER TABLE resource RENAME TO attachment;
DROP INDEX IF EXISTS idx_resource_creator_id;
CREATE INDEX idx_attachment_creator_id ON attachment (creator_id);
DROP INDEX IF EXISTS idx_resource_memo_id;
CREATE INDEX idx_attachment_memo_id ON attachment (memo_id);
"

# 0.26/01 - drop memo_organizer
sqlite3 "$DB" "DROP TABLE IF EXISTS memo_organizer;"

# 0.26/02 - drop old indexes
sqlite3 "$DB" "
DROP INDEX IF EXISTS idx_user_username;
DROP INDEX IF EXISTS idx_memo_creator_id;
DROP INDEX IF EXISTS idx_attachment_creator_id;
DROP INDEX IF EXISTS idx_attachment_memo_id;
"

# 0.26/03 - alter user table
sqlite3 "$DB" "
ALTER TABLE user RENAME TO user_old;
CREATE TABLE user (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  updated_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  row_status TEXT NOT NULL CHECK (row_status IN ('NORMAL', 'ARCHIVED')) DEFAULT 'NORMAL',
  username TEXT NOT NULL UNIQUE,
  role TEXT NOT NULL DEFAULT 'USER',
  email TEXT NOT NULL DEFAULT '',
  nickname TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  avatar_url TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT ''
);
INSERT INTO user (id, created_ts, updated_ts, row_status, username, role, email, nickname, password_hash, avatar_url, description)
SELECT id, created_ts, updated_ts, row_status, username, role, email, nickname, password_hash, avatar_url, description FROM user_old;
DROP TABLE user_old;
"

# 0.26/04 - HOST to ADMIN
sqlite3 "$DB" "UPDATE user SET role = 'ADMIN' WHERE role = 'HOST';"

echo "=== Migration complete ==="
```

- [ ] **Step 4: Verify post-migration state**

```bash
DB="/tmp/memos_migration_test/memos_prod.db"
echo "=== tables ===" && sqlite3 "$DB" ".tables"
echo "=== users ===" && sqlite3 "$DB" "SELECT id, username, role FROM user;"
echo "=== attachment count ===" && sqlite3 "$DB" "SELECT count(*) FROM attachment;"
echo "=== memo count ===" && sqlite3 "$DB" "SELECT count(*) FROM memo;"
echo "=== attachment blob check ===" && sqlite3 "$DB" "SELECT id, length(blob), size FROM attachment LIMIT 3;"
```

Expected:
- Tables: `attachment` (not `resource`), no `memo_organizer`
- Users: panqiang=ADMIN (was HOST), carol=ADMIN
- attachment: 11 records, blob lengths match size
- memo: 17 records

- [ ] **Step 5: Clean up test database**

```bash
rm -rf /tmp/memos_migration_test/
```

---

### Task 6: Verify Go build and frontend build

**Files:**
- None modified (build verification only)

- [ ] **Step 1: Verify Go backend compiles**

```bash
go build -o /dev/null ./cmd/memos
```

Expected: exits 0, no errors.

- [ ] **Step 2: Verify frontend builds**

```bash
cd web && pnpm install --frozen-lockfile && pnpm release
```

Expected: successful build, output in `server/router/frontend/dist/`.

If `pnpm install` fails (lockfile mismatch after cherry-pick), run:

```bash
cd web && pnpm install && pnpm release
```

- [ ] **Step 3: Run backend tests (if they pass on v0.26.2)**

```bash
go test ./store/... ./plugin/... ./internal/... -count=1 -short
```

Expected: tests pass. Some may be skipped (test containers for MySQL/PostgreSQL). SQLite tests should pass.

---

## Chunk 3: CI/CD Setup

### Task 7: Create fork Docker build workflow

**Files:**
- Create: `.github/workflows/build-fork-image.yml` (based on `build-stable-image.yml`)

- [ ] **Step 1: Create the workflow file**

Copy `build-stable-image.yml` and modify. The complete file:

```yaml
name: Build Fork Image

on:
  push:
    tags:
      - "v*.*.*-ds.*"

jobs:
  prepare:
    runs-on: ubuntu-latest
    outputs:
      version: ${{ steps.version.outputs.version }}
    steps:
      - name: Extract version
        id: version
        run: |
          echo "version=${GITHUB_REF_NAME#v}" >> $GITHUB_OUTPUT

  build-frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: pnpm/action-setup@v4.2.0
        with:
          version: 10
      - uses: actions/setup-node@v6
        with:
          node-version: "22"
          cache: pnpm
          cache-dependency-path: "web/pnpm-lock.yaml"
      - name: Get pnpm store directory
        id: pnpm-cache
        shell: bash
        run: echo "STORE_PATH=$(pnpm store path)" >> $GITHUB_OUTPUT
      - name: Setup pnpm cache
        uses: actions/cache@v5
        with:
          path: ${{ steps.pnpm-cache.outputs.STORE_PATH }}
          key: ${{ runner.os }}-pnpm-store-${{ hashFiles('web/pnpm-lock.yaml') }}
          restore-keys: ${{ runner.os }}-pnpm-store-
      - run: pnpm install --frozen-lockfile
        working-directory: web
      - name: Run frontend build
        run: pnpm release
        working-directory: web

      - name: Upload frontend artifacts
        uses: actions/upload-artifact@v6
        with:
          name: frontend-dist
          path: server/router/frontend/dist
          retention-days: 1

  build-push:
    needs: [prepare, build-frontend]
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    strategy:
      fail-fast: false
      matrix:
        platform:
          - linux/amd64
          - linux/arm/v7
          - linux/arm64
    steps:
      - uses: actions/checkout@v6

      - name: Download frontend artifacts
        uses: actions/download-artifact@v7
        with:
          name: frontend-dist
          path: server/router/frontend/dist

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKER_HUB_USERNAME }}
          password: ${{ secrets.DOCKER_HUB_TOKEN }}

      - name: Build and push by digest
        id: build
        uses: docker/build-push-action@v6
        with:
          context: .
          file: ./scripts/Dockerfile
          platforms: ${{ matrix.platform }}
          cache-from: type=gha,scope=build-${{ matrix.platform }}
          cache-to: type=gha,mode=max,scope=build-${{ matrix.platform }}
          outputs: type=image,name=deepshape-ai/memos,push-by-digest=true,name-canonical=true,push=true

      - name: Export digest
        run: |
          mkdir -p /tmp/digests
          digest="${{ steps.build.outputs.digest }}"
          touch "/tmp/digests/${digest#sha256:}"

      - name: Upload digest
        uses: actions/upload-artifact@v6
        with:
          name: digests-${{ strategy.job-index }}
          path: /tmp/digests/*
          if-no-files-found: error
          retention-days: 1

  merge:
    needs: [prepare, build-push]
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - name: Download digests
        uses: actions/download-artifact@v7
        with:
          pattern: digests-*
          merge-multiple: true
          path: /tmp/digests

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Docker meta
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: |
            deepshape-ai/memos
          tags: |
            type=raw,value=${{ needs.prepare.outputs.version }}
            type=raw,value=latest
          flavor: |
            latest=false
          labels: |
            org.opencontainers.image.version=${{ needs.prepare.outputs.version }}

      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKER_HUB_USERNAME }}
          password: ${{ secrets.DOCKER_HUB_TOKEN }}

      - name: Create manifest list and push
        working-directory: /tmp/digests
        run: |
          docker buildx imagetools create $(jq -cr '.tags | map("-t " + .) | join(" ")' <<< "$DOCKER_METADATA_OUTPUT_JSON") \
            $(printf 'deepshape-ai/memos@sha256:%s ' *)
        env:
          DOCKER_METADATA_OUTPUT_JSON: ${{ steps.meta.outputs.json }}

      - name: Inspect image
        run: |
          docker buildx imagetools inspect deepshape-ai/memos:${{ needs.prepare.outputs.version }}
```

Key differences from upstream `build-stable-image.yml`:
- Trigger: `v*.*.*-ds.*` tags only (not upstream tags)
- Image: `deepshape-ai/memos` (not `neosmemo/memos`)
- Removed GHCR push (GitHub Container Registry)
- Tags: `{version}` + `latest` (not `stable` / `major.minor`)
- Removed `release/**` branch trigger

- [ ] **Step 2: Disable upstream workflow triggers for fork**

The existing `build-stable-image.yml` triggers on `v*.*.*` tags, which would also match our `v0.26.2-ds.20260311` tag. Either:

Option A (recommended): Add a condition to skip fork tags:

In `build-stable-image.yml`, add to each job:

```yaml
if: "!contains(github.ref_name, '-ds.')"
```

Option B: Delete `build-stable-image.yml` entirely (if we never need upstream-style builds).

- [ ] **Step 3: Commit CI changes (wait for user confirmation)**

```bash
git add .github/workflows/build-fork-image.yml
git add .github/workflows/build-stable-image.yml  # if modified
git commit -m "ci: add fork Docker image build workflow for deepshape-ai/memos"
```

---

### Task 8: Create docker-compose.yml template

**Files:**
- Create: `deploy/docker-compose.yml`

- [ ] **Step 1: Create deploy directory and compose file**

```yaml
# deploy/docker-compose.yml
# Production deployment template for deepshape-ai/memos fork.
# Copy to server, adjust image tag, then: docker compose up -d
services:
  memos:
    image: deepshape-ai/memos:0.26.2-ds.20260311
    container_name: memos
    restart: unless-stopped
    ports:
      - "5230:5230"
    volumes:
      - ./data:/var/opt/memos
    environment:
      - TZ=Asia/Shanghai
```

- [ ] **Step 2: Commit (wait for user confirmation)**

```bash
git add deploy/docker-compose.yml
git commit -m "chore: add production docker-compose template"
```

---

## Chunk 4: Final Cleanup & Verification

### Task 9: Workspace cleanup and final verification

**Files:**
- Verify: `.gitignore` covers `resources/.memos/`
- Delete: `resources/.memos_test_backup/` (if still present)

- [ ] **Step 1: Verify .gitignore**

```bash
grep '.memos' .gitignore
```

If `resources/.memos/` is not covered, add it:

```
resources/.memos/
```

- [ ] **Step 2: Remove test artifacts**

```bash
rm -rf resources/.memos_test_backup/
```

- [ ] **Step 3: Final git status check**

```bash
git status
git log --oneline -8
git tag -l 'v0.26.2-ds*'
```

Expected:
- Clean working tree (no untracked files except intentionally ignored ones)
- Commit history: v0.26.2 base → daily-log → readme → mcp → mcp-improve → version/cleanup → ci → compose
- Tag: `v0.26.2-ds.20260311`

- [ ] **Step 4: Move tag to final commit (if CI/compose commits were added after initial tag)**

```bash
git tag -d v0.26.2-ds.20260311
git tag v0.26.2-ds.20260311
```

- [ ] **Step 5: Summary checklist**

```
[ ] main branch based on v0.26.2 with all fork features
[ ] Version: 0.26.2-ds.20260311
[ ] No 0.27/ migration files
[ ] LATEST.sql matches v0.26.2 schema
[ ] Go backend compiles
[ ] Frontend builds
[ ] Migration tested against production DB copy
[ ] CI workflow ready for deepshape-ai/memos
[ ] docker-compose.yml template ready
[ ] Clean git status
[ ] Tag v0.26.2-ds.20260311 on final commit
```

---

## Post-Plan: Production Deployment

**Not part of this plan.** After the user pushes to origin and the CI builds the Docker image, the production migration follows the procedure in the design spec (Section 4). Summary:

```bash
# On production server:
# 1. Backup: cp -r data data.bak.$(date +%Y%m%d%H%M)
# 2. Update docker-compose.yml image tag
# 3. docker compose pull && docker compose down && docker compose up -d
# 4. Verify: docker logs memos | grep migration
# 5. Test: login, check memos, check attachments, test daily-log, test MCP
```
