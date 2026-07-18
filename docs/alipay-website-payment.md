# 支付宝电脑网站支付

本分支为 New API 内置前端增加支付宝开放平台“电脑网站支付”原生直连，不依赖易支付网关。

## 上线前准备

1. 在支付宝开放平台应用中签约并启用“电脑网站支付”。
2. 准备应用 AppID、应用私钥和支付宝公钥。不要填写应用公钥，也不要在聊天、前端代码或日志中保存私钥。
3. 将 New API 的服务器地址设置为用户实际访问的公网入口。当前 IP 部署计划应填写 `http://8.218.169.39`，不要填写容器内地址或 `localhost`。
4. 确保公网可以访问 `POST /api/alipay/notify`。该路由不要求登录，但会校验支付宝 RSA2 签名、AppID、订单号、支付提供方、订单状态和金额。

## New API 后台配置

使用 root 管理员进入“系统设置 -> 计费设置 -> 支付网关 -> 支付宝”：

- `Alipay AppID`：支付宝开放平台应用 AppID。
- `应用私钥`：生成应用公钥时配套的 RSA2 私钥。
- `支付宝公钥`：支付宝开放平台显示的支付宝公钥。
- `沙箱模式`：正式收款时关闭，只有使用支付宝沙箱账号和沙箱密钥时开启。
- `启用支付宝`：前三项保存成功后再开启。

支付通知地址由服务端自动生成：

```text
<ServerAddress>/api/alipay/notify
```

用户付款后的同步返回地址由服务端自动生成，最终回到 New API 内置钱包页面。同步返回不会入账，只有通过验签和订单校验的异步通知才会增加余额。

## 宝塔与 Docker

当前需要构建并替换为本分支的自定义 New API 镜像，不能继续使用未包含本改动的上游原版镜像。继续复用现有数据库和数据卷，不要重新创建或清空数据库。

Nginx 必须完整转发 `/api/`，并保留原始主机和客户端协议头。公网入口使用 80 端口时，New API 的服务器地址填写 `http://8.218.169.39`。

### 当前公网状态

2026-07-14 的只读检查结果：

- `http://8.218.169.39:3000/api/status` 返回 200，线上版本为 `v1.0.0-rc.21`。
- `http://8.218.169.39/api/status` 返回 Nginx 404，说明 80 端口还没有代理到 New API。
- 线上 `ServerAddress` 仍为 `http://localhost:3000`，必须在真实支付前修改。
- 线上 Logo URL 为 `http://8.218.169.39/benefit-api-logo.png`；80 端口代理完成后，该静态资源才可访问。

### 构建自定义镜像

将本地发布包上传并解压到服务器的 `/www/wwwroot/benefit-api-source`，进入源码目录后执行：

```bash
cd /www/wwwroot/benefit-api-source
docker build --pull -t benefit-api:newapi-v1.0.0-rc.21-alipay-1 .
```

构建完成后先确认镜像存在，不要立即删除任何旧镜像：

```bash
docker image inspect benefit-api:newapi-v1.0.0-rc.21-alipay-1
```

### 备份与替换

先在宝塔中找到当前 New API 使用的 Compose 文件、数据库类型和数据卷。备份当前 Compose 文件，并根据实际数据库执行一致性备份：SQLite 应在短暂停止 New API 容器后复制数据库文件；MySQL 或 PostgreSQL 应使用对应的 dump 工具。备份文件不得放进源码构建目录，也不要把数据库密码或支付宝私钥发送到聊天中。

只将现有 Compose 中 New API 服务的镜像改为：

```yaml
image: benefit-api:newapi-v1.0.0-rc.21-alipay-1
```

保留现有服务名、端口、环境变量、数据库连接、网络和所有数据卷。随后仅重建 New API 服务；将 `<compose-file>` 和 `<service-name>` 替换为服务器上的真实值：

```bash
docker compose -f <compose-file> up -d --no-deps <service-name>
docker logs --tail 100 <container-name>
curl -i http://127.0.0.1:3000/api/status
```

如果服务器使用旧版 `docker-compose` 命令，则将上面的 `docker compose` 改为 `docker-compose`。

### 宝塔 Nginx

80 端口站点应将所有路径反向代理到 `127.0.0.1:3000`。可在宝塔网站的反向代理配置中使用以下核心配置：

```nginx
location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 600s;
    proxy_send_timeout 600s;
    proxy_buffering off;
}
```

保存并重载 Nginx 后验证：

```bash
curl -i http://8.218.169.39/api/status
curl -I http://8.218.169.39/benefit-api-logo.png
```

两个地址都正常后，再将 New API 后台的服务器地址保存为 `http://8.218.169.39`。

## 验收

1. 使用非生产金额或最小金额创建一笔订单。
2. 确认浏览器进入支付宝电脑网站收银台。
3. 付款后确认订单仅增加一次余额。
4. 在后台充值记录确认支付提供方为 `alipay_native`。
5. 重复投递同一通知时，余额不得再次增加。

如果支付宝拒绝纯 IP 或 HTTP 回调，应先配置已备案域名和 HTTPS，再将服务器地址切换为该 HTTPS 入口；不要绕过回调校验。
