# 群晖本地打包发布 SOP

本文档用于当前 `verychao/sub2api` 的实际发布链路约定。

目标不是追求“全自动”，而是在群晖外网不稳定、无法稳定拉取 GitHub/GHCR 镜像的前提下，采用一条稳定、可回滚、可追溯的发布流程。

## 适用场景

适用于以下条件：

- 群晖无法稳定访问 GitHub / GHCR
- GitHub Actions 可以正常构建，但群晖不能可靠拉取镜像
- 最终发布依赖本地导出镜像 tar 再上传群晖
- 需要明确区分测试环境和生产环境

## 链路定位

后续默认采用三段式链路：

1. GitHub：代码托管、CI 验证、版本锚点
2. 本地开发机：镜像准备、`docker save` 导出、上传中转
3. 群晖：导入镜像、测试部署、验收、生产切换

结论：

- GitHub 不再承担“直接部署到群晖”的主职责
- 本地打包并上传群晖，作为主发布方式
- 群晖测试环境先验收，再决定是否切生产

## 环境约定

当前环境建议固定为：

### 1. 本地开发环境

- 用途：改代码、构建、验证、导出镜像 tar
- 不作为正式运行环境

### 2. 群晖测试环境

- 容器名：`sub2api-app-test`
- 用途：验证新功能和新镜像
- 不直接覆盖生产容器

### 3. 群晖生产环境

- 容器名：`sub2api-app`
- 用途：正式对外服务
- 只有测试验收通过后才允许切换

## GitHub 的职责

GitHub 继续承担以下职责：

1. 保存所有代码和提交历史
2. 运行 CI，确认代码能通过基本验证
3. 生成镜像构建定义，作为可复现依据
4. 作为版本追溯和回滚锚点

GitHub 不再默认承担：

- 群晖直接拉取镜像并部署

## 推荐发布主流程

### 阶段 A：本地开发与验证

1. 在本地仓库完成代码修改
2. 本地执行必要验证
3. 推送到 GitHub 分支
4. 等待 CI 通过

建议至少确认：

- 前端 build 通过
- 后端 CI 通过
- 安全扫描无阻塞问题

### 阶段 B：准备可部署镜像

如果 GHCR 可访问，可优先使用 GitHub Actions 已构建镜像：

```bash
docker pull --platform linux/amd64 ghcr.io/verychao/sub2api:<tag>
```

如果 GHCR 不稳定，也可以在本地直接构建：

```bash
docker build -f deploy/Dockerfile -t sub2api:<tag> .
```

然后统一导出 tar：

```bash
docker save -o "/path/to/sub2api-<tag>-amd64.tar" <image>
```

建议命名规范：

- `sub2api-<branch>-<shortsha>-amd64.tar`

例如：

- `sub2api-feature-platform-usage-sync-9c54ddac-amd64.tar`

## 群晖测试部署流程

### 1. 上传 tar 到群晖

将导出的 tar 上传到群晖文件系统。

### 2. 导入镜像

在群晖 Container Manager 中执行：

- 映像/镜像
- 导入
- 从文件选择 tar

### 3. 更新测试容器

始终优先更新测试容器：

- 目标容器：`sub2api-app-test`

原则：

- 不直接覆盖 `sub2api-app`
- 旧测试容器先保留，便于回滚

推荐做法：

1. 停掉旧测试容器
2. 将旧测试容器改名为备份
3. 用新镜像重建新的 `sub2api-app-test`
4. 保持原测试端口和必要环境变量不变

## 验收流程

测试环境启动后，至少验证：

1. 页面可正常打开
2. 容器状态 `healthy`
3. 日志无数据库/Redis/迁移类阻塞报错
4. 本次功能点通过验收

建议通过以下两类证据确认：

### 容器侧

```bash
docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}'
docker logs --tail 100 sub2api-app-test
```

### 页面侧

- 功能截图
- 关键交互截图
- 新增字段或数据展示截图

## 回滚流程

如果测试镜像异常，优先回滚测试容器，不影响生产。

推荐策略：

1. 保留旧测试容器 `sub2api-app-test-old`
2. 新测试容器失败时，删除新容器
3. 将旧测试容器改回原名并启动

如果已经删除旧测试容器，则重新导入上一版 tar。

## 生产切换原则

生产切换不应和开发验证混在一起。

建议规则：

1. 只有测试环境通过验收后，才允许准备生产切换
2. 生产切换使用已验证通过的同一份镜像/tar
3. 生产切换前保留原生产容器或原镜像，确保可回滚

## 命名与追溯规范

每次准备发布时，至少记录以下信息：

1. Git 分支名
2. Commit SHA
3. 导出的 tar 文件名
4. 群晖测试容器所用镜像 tag
5. 验收结论

建议在发布记录中使用如下格式：

```text
branch: feature/platform-usage-sync
commit: 9c54ddac
image: ghcr.io/verychao/sub2api:feature-platform-usage-sync
tar: sub2api-feature-platform-usage-sync-9c54ddac-amd64.tar
target: sub2api-app-test
result: passed
```

## 是否保留 GitHub Branch Image

保留，原因：

1. 它仍然是标准构建定义
2. 它可以证明分支代码能构出镜像
3. 未来如果群晖网络恢复，可以重新直接拉 GHCR

但要明确：

- Branch Image 不是当前群晖部署的主入口
- 当前主入口仍然是本地导出 tar 上传群晖

## 当前推荐工作流

后续默认按下面执行：

1. 本地开发改代码
2. 本地验证
3. 推 GitHub 分支
4. 等 CI 通过
5. 本地拉取或构建目标镜像
6. `docker save` 导出 tar
7. 上传群晖
8. 群晖导入镜像
9. 更新 `sub2api-app-test`
10. 页面验收
11. 验收通过后再考虑生产切换

这条流程比“依赖群晖直接拉远端镜像”更符合当前环境约束，应作为默认主链路执行。
