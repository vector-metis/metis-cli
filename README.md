# Metis CLI

Metis 应用开发 CLI，使用 Go 编写，用于初始化应用工作目录、打包 MPK、校验包和检查包元数据。

## 安装

```bash
npm install -g @vector-metis/cli
```

也可以从 GitHub Releases 下载对应平台的原生二进制。npm 包不会在 `postinstall` 阶段临时下载二进制；平台包由同一次 CI 构建生成。

## 命令

```text
metis init [目录]
metis pack <工作目录> --output <文件.mpk>
metis validate <工作目录或文件.mpk>
metis inspect <工作目录或文件.mpk>
```

## Manifest 挂载与 Overlay

应用运行时由平台为每个安装分配独立的 scope。Manifest 中的沙箱挂载使用 `source + subpath`：`source` 是
`program`、`config`、`data`、`log`、`tmp` 或 `overlay` 之一，`subpath` 是根目录内的规范化相对路径；应用不需要知道 Worker 的宿主路径。

常驻 service 必须在 manifest 中声明生命周期，源 Compose 不得声明 `restart` 或 `x-metis`：

```yaml
services:
  web:
    lifecycle: {restart: unless-stopped}
```

包内的静态文件必须位于 `overlay/` 下，manifest 通过 `source: overlay` 和 `subpath` 挂载。例如：

```yaml
services:
  web:
    mounts:
      - {source: overlay, subpath: config.yaml, target: /etc/app/config.yaml, read_only: true}
      - {source: overlay, subpath: static, target: /usr/share/nginx/html, read_only: true}
```

`source: ./config.yaml`、绝对路径、`..` 路径、可写 overlay 挂载和 `${METIS_DIR_*}` 旧宿主目录占位符都会被
`validate`/`pack` 拒绝。镜像声明的 volume target 也必须有 manifest 挂载绑定。旧包不会自动改写，修改后请重新打包。

## 开发

```bash
go test ./...
go run ./cmd/metis --help
```

## 许可证

Apache-2.0，见 [LICENSE](LICENSE)。
