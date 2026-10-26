# 墨阅书屋 v1.0.0 —— 首发开源 Release 说明草稿

> 本文件为仓库首次公开发布（GitHub Release）的文案草稿，可直接作为 Release Notes 使用。
> 发布方式：打 `v1.0.0` tag 并推送，GitHub Actions（`.github/workflows/release.yml`）会自动交叉编译并打包
> `amd64` / `arm64` 两个架构的 `.fpk` 上传到 Release。
> **注意**：tag 会同步覆盖 `fnos/moyue/manifest` 的 `version` 字段与前端 `APP_VERSION`，请保证 tag 与本次发布版本一致。

## 这是什么

**墨阅书屋** 是一个部署在自托管环境（家庭服务器 / 飞牛 fnOS / 任意 Linux 主机）上的**个人图书管理与阅读中心**。
所有数据（书籍文件、数据库、配置）均保存在你自己的设备上，不上传任何云端。

- 后端：Go + Gin + GORM + 纯 Go SQLite（`glebarez/sqlite`），前端经 `go:embed` 内嵌为**单二进制**，零外部依赖。
- 前端：Vue 3 + Element Plus + Vite，集成 foliate-js（EPUB/AZW3/MOBI 阅读）、pdfjs-dist（PDF）、Edge TTS（AI 听书）。

## 核心功能

- **多格式书库**：拖拽批量导入 EPUB / MOBI / AZW3 / PDF / Markdown / TXT，自动识别书名、作者、出版社；书架搜索、收藏夹、阅读统计。
- **沉浸阅读**：目录导航（TXT 自动识别「第N章 / 第一章 / Chapter N」等章节标题）、翻页、字体字号与主题设置、划线高亮、批注笔记，阅读进度自动保存。
- **AI 听书**：一键把电子书分段合成为有声书，自动跳过目录、断点续传，支持暂停/恢复与试听；生成后全局后台播放，锁屏可用系统媒体面板控制。
- **读后感**：内置读后感阅读视图，记录心得、查看字数与阅读时长。
- **轻松分享**：图书与有声书一键生成分享链接，手机、平板随时访问。

## 三种部署方式

### 1. 单二进制（最简单）
```bash
# 构建（前端资源经 go:embed 内嵌，需先生成前端产物）
cd frontend && npm install && npm run build && cp -r dist ../backend/html/dist
cd ../backend && CGO_ENABLED=0 go build -o moyue-server .
./moyue-server
```
数据默认落在二进制同级的 `data/` 目录，浏览器打开 `http://<host>:18080`。

### 2. Docker
```bash
docker compose up -d --build          # 数据持久化到宿主机 ./data
# 或纯 Docker
docker build -t moyue-books .
docker run -d -p 18080:18080 -v "$(pwd)/data:/app/data" moyue-books
```
可用环境变量：`MB_DATA_DIR`、`PORT`、`EDGE_TTS_PROXY` / `HTTPS_PROXY`、`FEEDBACK_MAIL_ACCOUNT` / `FEEDBACK_MAIL_AUTH`。

### 3. 飞牛 fnOS（.fpk）
在 Release 附件下载对应架构的 `moyue_<version>_<platform>.fpk`，于飞牛「应用中心 → 手动安装」导入即可；应用支持 TCP 端口直连与统一网关两种入口。

## 发布前清单

- [ ] 关联远程仓库：`git remote add origin <地址>`
- [ ] 替换 `.github/ISSUE_TEMPLATE/config.yml` 中的安全漏洞反馈占位链接 `OWNER/REPO` 为实际仓库路径
- [ ] 确认 `fnos/moyue/manifest` 的 `maintainer` / `distributor` / `labels` 等信息是否需要调整
- [ ] 本地或 CI 跑通 `./fnos/build.sh amd64 --pack` 与 `arm64 --pack`，确认 `.fpk` 可生成
- [ ] 准备 `docs/screenshots/` 下的 4 张应用截图（bookshelf / reader / edit / settings），提交后 README 自动渲染
- [ ] 推送 tag：`git tag v1.0.0 && git push origin v1.0.0 --tags`

## 许可证

以 [MIT 许可证](../../LICENSE) 开源。欢迎 Issue 与 Pull Request，详见 [CONTRIBUTING.md](../../CONTRIBUTING.md)。

## 已知限制（首发版）

- 问题反馈邮件通道需自行配置 163 SMTP 环境变量后才会启用（源码不含任何凭据）。
- Edge TTS 依赖微软接口出网，受限网络可通过 `EDGE_TTS_PROXY` / `HTTPS_PROXY` 代理。
- 阅读统计、PDF 批注、移动端部分交互仍在完善中（详见 README「开发计划」）。
