# Design: Migration to v0.26.2 Stable & Fork Release Strategy

Date: 2026-03-11
Status: Draft v2 (post-review)

## Context

This project is forked from [usememos/memos](https://github.com/usememos/memos). Four commits add a daily-log system and MCP support atop the upstream `main` branch (v0.27.0-dev, unreleased). Production runs `neosmemo/memos:0.25.3` with an SQLite database at schema version 0.25.1.

Problem: v0.27.0 is unreleased and unstable. Basing the fork on it risks inheriting bugs and facing unpredictable breaking changes when syncing upstream.

Decision: Rebase all fork work onto `v0.26.2` (latest upstream stable release), then migrate production safely.

Driver: SQLite only. This deployment does not use MySQL or PostgreSQL.

---

## 1. Versioning Scheme

### Format: `{upstream}-ds.{YYYYMMDD}`

- `{upstream}` — upstream base version (always tracks a stable release tag, e.g., `0.26.2`)
- `-ds` — deepshape fork identifier
- `.{YYYYMMDD}` — fork release date (CalVer), naturally incrementing

### Examples

```
v0.26.2-ds.20260311    # First fork release: daily-log + MCP on v0.26.2 base
v0.26.2-ds.20260315    # Bug fix or new feature on same upstream base
v0.26.2-ds.20260401    # Another iteration

# Sync upstream v0.28.0 stable → upstream portion changes
v0.28.0-ds.20260501    # Rebased onto upstream v0.28.0
v0.28.0-ds.20260510    # Continued iteration on v0.28.0 base
```

### Rules

1. **Upstream portion** changes only when rebasing onto a new upstream stable tag.
2. **CalVer portion** uses the release date. Same-day re-release uses suffix: `v0.26.2-ds.20260311.2`.
3. **Syncing upstream is optional** — only when needed (new features, security fixes). Skipping versions is fine (e.g., 0.26.2 → 0.28.0 directly).
4. Git tags: `v0.26.2-ds.20260311`.
5. Docker images: `deepshape-ai/memos:0.26.2-ds.20260311` and `deepshape-ai/memos:latest`.

### Version Constant vs Schema Version (Important)

The memos codebase has two distinct version concepts:

1. **`internal/version/version.go` → `Version`** — display version, used for Docker tags and user-facing info. Set to `"0.26.2-ds.20260311"`.

2. **DB schema version** — derived by `GetCurrentSchemaVersion()` from migration file paths, not from the `Version` constant. With v0.26.2 migration files (0.26/00 through 0.26/04), this resolves to `"0.26.5"`. Stored in `system_setting` after migration.

These two values intentionally differ. The `Version` constant does not participate in migration logic — the migrator uses `GetMinorVersion(Version)` only to locate the migration directory (`0.26/`), then derives the target schema version from file names within it. Since `GetMinorVersion("0.26.2-ds.20260311")` splits by `.` and returns `"0.26"`, the directory lookup works correctly.

**Semver note:** Per the semver spec, pre-release identifiers sort lower than the release version. This does not affect migration because the migrator never compares the `Version` constant against schema versions.

---

## 2. Branch Reconstruction

### Goal

Clean branch `main` based on `v0.26.2` with all fork features applied.

### Git Operations

```bash
git fetch upstream --tags

# Create new branch from stable tag
git checkout -b main-stable v0.26.2

# Cherry-pick fork commits in order
git cherry-pick ebb0f583    # feat: add daily-log system
# → Resolve 2 conflicts (see below)
git cherry-pick 38a472ba    # chore(doc): update readme
git cherry-pick f27f85b7    # feat(mcp): add /mcp ability
git cherry-pick 952c6f43    # feat: improve mcp with mcp-builder skill
```

### Post-Cherry-Pick Adjustments (single commit)

```bash
# 1. Update version
#    internal/version/version.go → "0.26.2-ds.20260311"

# 2. Delete 0.27 migration files (they belong to unreleased upstream)
rm -rf store/migration/sqlite/0.27/
rm -rf store/migration/mysql/0.27/
rm -rf store/migration/postgres/0.27/

# 3. Verify LATEST.sql matches v0.26.2 schema
#    The LATEST.sql from v0.26.2 base should already be correct.
#    Confirm: no `uid` column in `idp` table, no `STORAGE` insert.

# 4. go mod tidy

# 5. Commit adjustments
git commit -am "chore: set version 0.26.2-ds.20260311 and remove unreleased 0.27 migrations"
```

### Replace main and tag

```bash
# Replace main with the new clean branch
git branch -D main
git branch -m main-stable main
git tag v0.26.2-ds.20260311

# Force push (destructive — requires explicit decision)
git push origin main --force-with-lease
git push origin v0.26.2-ds.20260311

# Clean up
git branch -D chore/daily-log-migration
git stash drop  # if any stashes exist
```

### Known Conflicts (Verified via worktree test)

Cherry-pick of `ebb0f583` produces exactly 2 file conflicts:

**`server/router/api/v1/v1.go`** — SSE endpoint + daily-log route registration.
v0.26.2 has neither SSE nor daily-log routes. Resolution: add both blocks (SSE handler + `RegisterDailyLogRoutes`) before the catch-all handler. Accept the incoming (fork) additions.

**`web/src/router/index.tsx`** — v0.27.0 introduced `lazyWithReload()` wrapper; v0.26.2 uses plain `lazy()`.
Resolution: register `DailyLog` route using v0.26.2's `lazy()` pattern.

MCP commits (`f27f85b7`, `952c6f43`) add new files under `server/router/mcp/` which does not exist in v0.26.2. These are pure file additions — no conflicts expected. `go.mod` adding `mcp-go` dependency may need a manual merge if surrounding lines differ.

---

## 3. Database Migration (Production)

### Schema Changes: 0.25.1 → 0.26.2

Five migration scripts applied automatically by the built-in migrator on startup.

Note: `0.25/00__remove_webhook.sql` produces file version `0.25.1`. Since the DB is already at schema `0.25.1`, `shouldApplyMigration()` evaluates `0.25.1 > 0.25.1` as false — this script is **skipped**.

| Step | File | Operation | Risk |
|------|------|-----------|------|
| 1 | `0.26/00__rename_resource_to_attachment.sql` | Rename `resource` → `attachment` + recreate indexes | None (all 11 resources stored as blobs in DB) |
| 2 | `0.26/01__drop_memo_organizer.sql` | `DROP TABLE IF EXISTS memo_organizer` | None (0 rows in production) |
| 3 | `0.26/02__drop_indexes.sql` | Drop old indexes | None |
| 4 | `0.26/03__alter_user_role.sql` | Recreate `user` table with tighter schema | Low (tested: all 2 users preserved with all fields intact) |
| 5 | `0.26/04__migrate_host_to_admin.sql` | `UPDATE user SET role='ADMIN' WHERE role='HOST'` | Low (panqiang: HOST→ADMIN, functionally equivalent) |

All 5 migrations run in a single atomic transaction. If any fails, the entire transaction rolls back and the DB stays at 0.25.1.

Tested locally against a copy of the production database (`resources/.memos/memos_prod.db`) — all steps succeed, all data preserved (2 users, 17 memos, 11 attachments, 1 relation, 1 reaction, 4 user_settings).

### Daily-Log Feature

No schema migration required. Daily logs are stored as regular memos with `payload.type = "DAILY_LOG"` (enum 100). The existing `memo` table and `payload` JSON column accommodate this without changes.

---

## 4. Production Migration Procedure

### Prerequisites

- New Docker image built and pushed: `deepshape-ai/memos:0.26.2-ds.20260311`
- SSH access to production server
- Current deployment: `neosmemo/memos:0.25.3` with data volume (e.g., `/opt/memos/data/`)

### Step-by-Step

```bash
# 1. Backup (mandatory — name includes timestamp for uniqueness)
ssh prod
cd /opt/memos
BACKUP_DIR="data.bak.$(date +%Y%m%d%H%M)"
cp -r data "$BACKUP_DIR"
echo "Backup created at: $BACKUP_DIR"

# 2. Create docker-compose.yml (first time only)
cat > docker-compose.yml << 'YAML'
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
YAML

# 3. Stop old container
docker stop memos && docker rm memos
# Or if already using compose: docker compose down

# 4. Pull new image and start
docker compose pull
docker compose up -d

# 5. Verify migration in logs
docker logs memos 2>&1 | grep -E "(migration|schema)"
# Expected output:
#   start migration currentSchemaVersion=0.25.1 targetSchemaVersion=0.26.5
#   applying migration file=...0.26/00__rename_resource_to_attachment.sql
#   applying migration file=...0.26/01__drop_memo_organizer.sql
#   applying migration file=...0.26/02__drop_indexes.sql
#   applying migration file=...0.26/03__alter_user_role.sql
#   applying migration file=...0.26/04__migrate_host_to_admin.sql
#   migration completed migrationsApplied=5

# 6. Verify data integrity — check ALL of these:
#   [ ] Log in as panqiang (now ADMIN role, was HOST)
#   [ ] All 17 memos present and readable
#   [ ] Images/attachments load correctly (11 items)
#   [ ] Create a new daily-log entry
#   [ ] Test MCP endpoint: curl -s http://localhost:5230/mcp (should return MCP protocol response)

# 7. Rollback procedure (if migration failed)
docker compose down
rm -rf data                    # Remove migrated (potentially corrupt) data
mv "$BACKUP_DIR" data          # Restore from the specific backup
docker run -d --name memos \
  -p 5230:5230 \
  -v $(pwd)/data:/var/opt/memos \
  neosmemo/memos:0.25.3        # Restart old version
```

### Estimated Downtime

Under 2 minutes. Migration applies 5 lightweight SQL statements in a single atomic transaction.

---

## 5. CI/CD: Docker Image Build

### Strategy

Fork the existing `build-stable-image.yml` to `build-fork-image.yml`. Key changes from upstream workflow:

| Item | Upstream | Fork |
|------|----------|------|
| Trigger | `tags: v*.*.*` | `tags: v*.*.*-ds.*` |
| Docker Hub image | `neosmemo/memos` | `deepshape-ai/memos` |
| GHCR image | `ghcr.io/usememos/memos` | Remove (or change to fork org) |
| Tag pattern | `stable`, `0.26`, `0.26.2` | `latest`, `0.26.2-ds.20260311` |

### Locations to update in the workflow

The following references to `neosmemo/memos` must all change to `deepshape-ai/memos`:

1. `build-push` job → `docker/build-push-action` → `outputs.name` (image name for digest push)
2. `merge` job → `docker/metadata-action` → `images` list
3. `merge` job → manifest creation `docker buildx imagetools create` command
4. `merge` job → `docker buildx imagetools inspect` commands
5. Docker Hub login secrets: `DOCKER_HUB_USERNAME` / `DOCKER_HUB_TOKEN` (configure in fork repo settings)

### Secrets Required

- `DOCKER_HUB_USERNAME` — Docker Hub account for `deepshape-ai` org
- `DOCKER_HUB_TOKEN` — Docker Hub access token with push permission

---

## 6. Ongoing Update Strategy

### 6.1 Syncing Upstream Stable Releases

Only sync with tagged stable releases. Never track upstream `main`.

```bash
# When upstream publishes a new stable tag (e.g., v0.28.0):
git fetch upstream --tags

# Review upstream changes before syncing
git log v0.26.2..v0.28.0 --oneline -- store/migration/  # DB migrations
git log v0.26.2..v0.28.0 --oneline -- proto/             # Proto changes
git diff v0.26.2..v0.28.0 -- store/                      # Store layer

# Create rebase branch
git checkout -b rebase/0.28.0 main

# Rebase the N fork commits (after version bump removal) onto new base
# Identify fork commits: everything after the version-bump commit
git rebase --onto v0.28.0 v0.26.2 rebase/0.28.0
# Note: v0.26.2 here is the original base tag. This replays all
# commits from v0.26.2..HEAD onto v0.28.0.

# Resolve conflicts, then:
# - Update version: 0.28.0-ds.YYYYMMDD (use today's date)
# - Remove any migration files beyond what v0.28.0 provides
# - Run go mod tidy
# - Test migration against a production DB copy
# - Merge to main, tag v0.28.0-ds.YYYYMMDD, push
```

### Decision Criteria for Upstream Sync

| Consideration | Action |
|---------------|--------|
| Upstream has new DB migrations | Test migration path against a production DB copy before releasing |
| Upstream changes proto definitions | Check for conflicts with daily-log proto (enum 100 chosen to avoid collision) |
| Upstream modifies store layer | Verify daily-log filter/query code still works |
| Upstream refactors MCP | Evaluate whether to adopt upstream MCP or keep fork's implementation |
| Upstream changes not needed | Skip the release; stay on current base until there's a reason to sync |

### 6.2 Server Update Procedure

```bash
# On production server:
cd /opt/memos

# 1. Backup
BACKUP_DIR="data.bak.$(date +%Y%m%d%H%M)"
cp -r data "$BACKUP_DIR"

# 2. Update image tag in docker-compose.yml
#    Change: image: deepshape-ai/memos:OLD_VERSION
#    To:     image: deepshape-ai/memos:NEW_VERSION

# 3. Pull and restart
docker compose pull
docker compose up -d

# 4. Verify
docker logs memos 2>&1 | tail -20

# 5. Clean up old backups after confirming stability (e.g., after 1 week)
```

### 6.3 Monitoring Upstream

- Watch upstream releases: `gh api repos/usememos/memos/releases/latest`
- Before syncing, review the release changelog and migration files
- Never sync with upstream `main` — only sync with tagged stable releases

---

## 7. Changelog: v0.26.2-ds.20260311

Based on upstream v0.26.2 stable. All changes below are fork-specific additions.

### New Features

**Daily Log System**

- New memo type `DAILY_LOG` (payload.type enum 100) for structured daily logging
- REST API: `PUT/GET/DELETE /api/v1/daily-logs/:date`, `GET /api/v1/daily-logs`
- gRPC: `SaveDailyLog`, `GetDailyLog`, `ListDailyLogs` RPCs
- One log per user per day with upsert semantics
- Content format: line-based with prefixes (`* ` done, `+ ` to-do, `- ` note, `? ` question)
- 36-hour edit window to accommodate all timezones (UTC-12 to UTC+14)
- Visibility fixed to PROTECTED (workspace-visible)
- Timezone-aware date handling (client sends local midnight, no server-side UTC truncation)
- SSE broadcasting for live UI updates
- Frontend: dedicated Daily Log page, home entry widget, activity calendar integration
- i18n: English and Simplified Chinese

**MCP (Model Context Protocol) Server**

- Streamable HTTP transport at `/mcp` endpoint (MCP spec 2025-03-26)
- Authentication: optional Bearer token (PAT or JWT)
- Memo tools: list, get, create, update, delete, search, comments (8 tools)
- Daily log tools: save, get, list (3 tools)
- Tag tools: list_tags (1 tool)
- Resources: `memo://memos/{uid}` template for markdown export
- Prompts: capture, daily_log, review (3 prompts)

### Upstream Base (v0.26.2)

Changes inherited from upstream v0.25.3 → v0.26.2:

- `resource` table renamed to `attachment` (API updated accordingly)
- `memo_organizer` table removed (unused)
- `HOST` user role replaced by `ADMIN` (functionally equivalent)
- User table recreated with stricter schema definition
- Various bug fixes and improvements (see [upstream release notes](https://github.com/usememos/memos/releases))

### Migration

- Automatic migration from v0.25.x schemas via built-in migrator
- 5 SQL migration scripts applied in a single atomic transaction
- No manual intervention required
- Tested against production database copy: zero data loss

### Breaking Changes

- Users with `HOST` role are automatically migrated to `ADMIN`
- The `resource` table/API is renamed to `attachment`
- Minimum upgrade path: v0.22.0+ → v0.26.2-ds.20260311 (pre-v0.22 must upgrade to v0.25.x first)

---

## 8. Workspace Cleanup

After branch reconstruction completes, the final workspace state:

1. **Single `main` branch** based on v0.26.2 with 5 clean commits (4 cherry-picks + 1 version/cleanup)
2. **Tag `v0.26.2-ds.20260311`** on the final commit
3. **Deleted**: branch `chore/daily-log-migration`, any stash entries
4. **Deleted**: `resources/.memos_test_backup/` (test artifacts from migration verification)
5. **Verified**: `resources/.memos/` in `.gitignore` (production data must not be committed)
6. **No 0.27/ migration files** in the tree
7. **Clean `git status`** — no uncommitted changes

---

## 9. Risk Summary

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Cherry-pick conflicts | Confirmed (2 files) | Low | Conflicts are trivial, verified in worktree test |
| Migration data loss | Very Low | High | Full backup + atomic transaction + tested on prod DB copy |
| MCP dependency issue | Low | Low | `mcp-go` is a pure addition, same `echo`/`connect` versions as v0.26.2 |
| Upstream proto enum collision | Very Low | Medium | Daily-log uses enum 100, far from upstream's current range (0-10) |
| Docker permission (non-root) | Low | Low | v0.26.2 already uses non-root; entrypoint handles UID migration |
| Version/schema mismatch confusion | Low | Low | Documented: Version constant (`0.26.2-ds.20260311`) is display-only; DB stores schema version (`0.26.5`) derived from migration files |
