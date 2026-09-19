# Changelog

## Unreleased

- 同步 `metis-sdk-contracts v0.1.6`：支持 `source + subpath`、manifest lifecycle、overlay 文件/目录挂载和 image volume 门禁。
- `metis inspect` 输出展开后的 manifest mounts 及镜像 volume 匹配事实。
- `metis init` 生成 manifest lifecycle，源 Compose 不再写入 `restart` 或 `x-metis`。

## 0.1.9

- 更新公开 `metis-sdk-contracts` 至 `v0.1.5`，修复规范化 overlay source 在安装计划中重复添加 `./` 的问题。

## 0.1.8

- 更新公开 `metis-sdk-contracts` 至 `v0.1.4`。
- 校验并打包时拒绝 `${METIS_DIR_*}` 旧宿主目录占位符，并保留 overlay 空目录。

## 0.1.7

- 更新公开 `metis-sdk-contracts` 至 `v0.1.3`，`metis validate` 与 `metis pack` 现在接受规范化的 `./overlay`、`./overlay/...` 挂载，并拒绝旧的 overlay source 与 `METIS_DIR_*` 规则。

## 0.1.6

- `metis init` 生成的常驻 Compose service 默认声明 `restart: unless-stopped`。

## 0.1.5

- 使用带 SemVer 依赖约束的公开契约 `metis-sdk-contracts v0.1.1`。

## 0.1.0

- 首次公开发布。
