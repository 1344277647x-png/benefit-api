# Benefit API GitHub 与宝塔部署

本文说明如何将 Benefit API 源码推送到公开 GitHub 仓库，由 GitHub Actions 构建 AMD64 Docker 镜像，再在宝塔服务器拉取运行。

## 仓库与镜像

- GitHub 仓库：`https://github.com/1344277647x-png/benefit-api`
- GHCR 镜像：`ghcr.io/1344277647x-png/benefit-api`
- 持续更新标签：`latest`
- 固定版本标签：`sha-<提交短哈希>`

生产环境应使用固定 `sha-*` 标签，便于确认运行版本和回滚。

## 首次发布

1. 在 GitHub 创建公开仓库 `1344277647x-png/benefit-api`。
2. 将本地 `main` 分支推送到该仓库。
3. 在仓库的 Actions 页面等待 `Publish Benefit API image` 成功。
4. 打开 GitHub Packages 中的 `benefit-api`，将 Package visibility 设置为 Public。
5. 从 Actions 摘要复制本次构建的 `sha-*` 标签。

工作流会运行后端测试、前端类型检查、lint、格式检查和生产构建，然后发布 `linux/amd64` 镜像，并用 `/api/status` 执行镜像冒烟测试。

## 宝塔服务器更新

更新前必须备份当前 Compose 文件和数据库。不要删除或重建 MySQL、Redis、`data`、`logs` 或其他数据卷。

在现有 `docker-compose.yml` 中只修改 `new-api` 服务的镜像，例如：

```yaml
services:
  new-api:
    image: ghcr.io/1344277647x-png/benefit-api:sha-abcdef0
```

进入现有 Compose 目录后执行：

```bash
docker compose pull new-api
docker compose up -d --no-deps new-api
docker compose ps
docker compose logs --tail=100 new-api
```

如果服务器使用旧版独立命令，将上述 `docker compose` 原样替换为 `docker-compose`。

## 验证

```bash
curl -fsS https://xbenefitapi.xyz/api/status
curl -fsS https://xbenefitapi.xyz/api/docs
```

随后在浏览器验证：

1. 首页、登录和控制台正常。
2. “文档”进入 `/docs`。
3. 管理员在“系统设置 → 站点设置 → 文档”保存 Markdown 后，公开文档立即更新。
4. “文档链接”填写 `/docs`，顶部导航和首页按钮均在站内打开。
5. 钱包、支付设置和 `/v1` API 保持正常。

## 回滚

将 Compose 中的镜像标签改回上一个确认可用的 `sha-*`，然后执行：

```bash
docker compose pull new-api
docker compose up -d --no-deps new-api
```

回滚只替换应用容器，不修改数据库和数据卷。
