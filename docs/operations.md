# Memos Fork 运维手册

## 1. 线上升级：neosmemo/memos:0.25.3 -> deepshape-ai/memos:0.26.2-ds.20260311

### 前置条件

- fork镜像已推送到 Docker Hub: `deepshape-ai/memos:0.26.2-ds.20260311`
- 线上当前运行: `neosmemo/memos:0.25.3`，数据目录挂载在宿主机上

### 升级步骤

```bash
ssh <prod-server>
cd /opt/memos  # 或实际部署目录

# 1. 备份数据
BACKUP="data.bak.$(date +%Y%m%d%H%M)"
cp -r data "$BACKUP"

# 2. 停止旧容器
docker stop memos && docker rm memos

# 3. 写入 docker-compose.yml（首次升级时创建）
cat > docker-compose.yml << 'EOF'
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
EOF

# 4. 拉取并启动
docker compose pull
docker compose up -d

# 5. 确认迁移成功
docker logs memos 2>&1 | grep -E "migration|schema"
# 预期输出：
#   start migration currentSchemaVersion=0.25.1 targetSchemaVersion=0.26.5
#   applying migration ...0.26/00__rename_resource_to_attachment.sql
#   applying migration ...0.26/01__drop_memo_organizer.sql
#   applying migration ...0.26/02__drop_indexes.sql
#   applying migration ...0.26/03__alter_user_role.sql
#   applying migration ...0.26/04__migrate_host_to_admin.sql
#   migration completed migrationsApplied=5
```

### 验证清单

```
[ ] 登录（panqiang 角色已从 HOST 变为 ADMIN，功能等价）
[ ] 所有 memo 可正常阅读
[ ] 图片附件正常显示
[ ] 创建一条 daily-log
[ ] MCP 端点可达: curl http://localhost:5230/mcp
```

### 回滚

```bash
docker compose down
rm -rf data
mv "$BACKUP" data
docker run -d --name memos -p 5230:5230 \
  -v $(pwd)/data:/var/opt/memos neosmemo/memos:0.25.3
```

### 迁移原理

应用启动时，内置 migrator 检测数据库 schema 版本（0.25.1），自动执行 5 条 SQL，在单个事务内完成。失败则整体回滚，数据库保持原状。

主要变更：`resource` 表重命名为 `attachment`，`memo_organizer` 表删除（无数据），`HOST` 角色合并为 `ADMIN`。所有附件以 blob 存储在 SQLite 中，重命名操作不影响数据。

daily-log 功能无需 schema 变更，复用 memo 表的 payload 字段。

---

## 2. 后续版本迭代更新

### 版本号规则

```
v{upstream基线}-ds.{YYYYMMDD}
```

- upstream 基线：跟踪的上游 stable 版本（如 0.26.2）
- ds.YYYYMMDD：fork 发版日期

示例：

```
v0.26.2-ds.20260311    当前版本
v0.26.2-ds.20260320    同基线下的迭代
v0.28.0-ds.20260601    同步上游 v0.28.0 后的首版
```

### 开发发版流程

```bash
# 开发完成后
# 1. 更新 internal/version/version.go 中的 Version 常量
# 2. 提交并打 tag
git tag v0.26.2-ds.20260320
git push origin main
git push origin v0.26.2-ds.20260320

# 3. GitHub Actions 自动构建并推送 Docker 镜像
#    触发条件：tag 匹配 v*.*.*-ds.*
#    产物：deepshape-ai/memos:0.26.2-ds.20260320 + deepshape-ai/memos:latest
```

### 同步上游 stable release

只跟踪上游 tagged release，不跟踪 main 分支。

```bash
git fetch upstream --tags

# 评估变更
git log v0.26.2..v0.28.0 --oneline -- store/migration/
git diff v0.26.2..v0.28.0 -- proto/ store/

# rebase fork commits
git checkout -b rebase/0.28.0 main
git rebase --onto v0.28.0 v0.26.2

# 解决冲突后：更新版本、go mod tidy、测试迁移、提交、打 tag
```

重点检查：新增 DB 迁移脚本、proto 定义变更（daily-log 使用 enum 100，远离上游范围）、store 层接口变更。

---

## 3. 线上持续部署

### 常规更新

```bash
ssh <prod-server>
cd /opt/memos

# 备份
cp -r data "data.bak.$(date +%Y%m%d%H%M)"

# 更新镜像 tag
sed -i 's|image:.*|image: deepshape-ai/memos:<NEW_VERSION>|' docker-compose.yml

# 部署
docker compose pull
docker compose up -d

# 验证
docker logs memos 2>&1 | tail -20
```

### 如果新版本包含 DB 迁移

migrator 会自动检测并执行。部署前先在本地用生产 DB 副本验证：

```bash
# 本地测试
cp <prod-db-copy> /tmp/test.db
# 启动新版本指向测试 DB，检查迁移日志
```

确认无误后再在线上执行。

### 备份清理

稳定运行一周后清理旧备份：

```bash
ls -d data.bak.* | head -n -1 | xargs rm -rf
```
