# Sub2API 群晖测试发布说明

本文档面向当前 `verychao/sub2api` 自定义版本，目标是把已完成的 `usage` 页面成果发布到群晖测试环境，而不直接覆盖生产。

当前建议使用以下代码版本：

- 功能提交：`20aa1a95 feat: refine usage page upstream balance visibility`
- 部署修正：`6c679959 build: increase frontend docker build memory`

## 发布原则

1. 先本地打包镜像，再上传群晖。
2. 先部署测试环境，再决定是否切生产。
3. 不把本机开发覆盖文件带上群晖。

本次明确不纳入发布版本的文件：

- `deploy/docker-compose.dev.local-override.yml`
- `frontend/package-lock.json`

## 推荐部署文件

群晖侧优先使用：

- `deploy/docker-compose.local.yml`

原因：

1. 数据目录直观，方便备份和迁移。
2. 比命名卷更适合 NAS 场景排障。
3. 可以明确区分测试环境和生产环境。

## 本地构建镜像

在仓库根目录执行：

```bash
docker build -t sub2api:usage-20aa1a95 -f Dockerfile .
```

建议 tag 命名包含功能或提交号，例如：

- `sub2api:usage-20aa1a95`
- `sub2api:custom-20aa1a95`

如果群晖是 `amd64`，本地也是 `amd64`，上面的命令即可。

如果你后续需要显式指定平台，可使用：

```bash
docker build --platform linux/amd64 -t sub2api:usage-20aa1a95 -f Dockerfile .
```

## 导出镜像 tar

```bash
docker save -o "sub2api-usage-20aa1a95-amd64.tar" sub2api:usage-20aa1a95
```

建议保留固定命名格式：

- `sub2api-<feature>-<shortsha>-amd64.tar`

例如：

- `sub2api-usage-20aa1a95-amd64.tar`

## 群晖测试环境目录建议

群晖测试环境建议单独放一套目录，例如：

```text
/volume1/docker/sub2api-test/
  .env
  docker-compose.local.yml
  data/
  postgres_data/
  redis_data/
```

不要和生产环境目录混用。

## 群晖测试环境最小 .env 模板

下面是一份最小可用模板，按实际情况替换：

```dotenv
BIND_HOST=0.0.0.0
SERVER_PORT=38081
SERVER_MODE=release
RUN_MODE=standard
TZ=Asia/Shanghai

POSTGRES_USER=sub2api
POSTGRES_PASSWORD=replace_with_strong_password
POSTGRES_DB=sub2api

REDIS_PASSWORD=
REDIS_DB=0

ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=replace_with_admin_password

JWT_SECRET=replace_with_fixed_jwt_secret
TOTP_ENCRYPTION_KEY=replace_with_fixed_totp_key
```

建议用下面命令生成密钥：

```bash
openssl rand -hex 32
```

至少生成并固定这三个值：

1. `POSTGRES_PASSWORD`
2. `JWT_SECRET`
3. `TOTP_ENCRYPTION_KEY`

## 群晖测试部署步骤

1. 上传 `sub2api-usage-20aa1a95-amd64.tar` 到群晖。
2. 在 Container Manager 中导入该镜像。
3. 把 `deploy/docker-compose.local.yml` 放到群晖测试目录。
4. 复制并填写 `.env`。
5. 创建目录：

```bash
mkdir -p data postgres_data redis_data
```

6. 把 `docker-compose.local.yml` 里的镜像改成你导入后的镜像 tag。

默认文件里是：

```yaml
image: weishaw/sub2api:latest
```

测试时要改成你的自定义镜像，例如：

```yaml
image: sub2api:usage-20aa1a95
```

7. 先启动测试环境，不要覆盖生产：

```bash
docker compose -f docker-compose.local.yml up -d
```

## 测试验收项

上线后至少检查：

1. 页面可以正常打开。
2. `sub2api`、`postgres`、`redis` 容器都正常启动。
3. `sub2api` 容器健康检查通过。
4. 日志里没有数据库连接、Redis 连接、迁移失败等阻塞错误。
5. `usage` 页面满足当前成果预期：
   - 默认时间范围是“今天”
   - 顶部余额展示正常
   - 顶部与下方余额概览数据一致
   - 手动刷新正常
   - 切筛选/翻页/时间范围不乱触发余额刷新

可用命令：

```bash
docker compose -f docker-compose.local.yml logs -f sub2api
docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}'
```

## 回滚建议

如果测试环境异常：

1. 不动生产环境。
2. 删除当前测试容器。
3. 重新导入上一版 tar 或切回上一版测试镜像。

如果后续需要切生产：

1. 必须使用已经在测试环境验收通过的同一份镜像。
2. 保留原生产容器或原镜像，确保可回滚。

## 当前推荐链路

后续默认按这条链路执行：

1. 本地完成代码和验证。
2. 本地构建自定义镜像。
3. `docker save` 导出 tar。
4. 上传群晖。
5. 群晖导入镜像。
6. 用 `docker-compose.local.yml` 部署测试环境。
7. 页面验收通过后，再决定是否切生产。
