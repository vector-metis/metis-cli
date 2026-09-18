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

## 应用目录与 Overlay

应用运行时由平台为每个安装分配独立的 scope。Manifest 中的沙箱 source 使用固定的逻辑目录名：`program`、`config`、`data`、`log` 和 `tmp`；应用不需要知道 Worker 的宿主路径。

包内的静态文件必须位于 `overlay/` 下，Manifest 挂载时只能使用规范化的相对 source：`./overlay` 或 `./overlay/...`。例如：

```yaml
services:
  web:
    mounts:
      - {source: ./overlay/config.yaml, target: /etc/app/config.yaml, read_only: true}
      - {source: ./overlay/static, target: /usr/share/nginx/html, read_only: true}
```

`./config.yaml`、`./overlay/../config.yaml`、绝对路径、可写 overlay 挂载和 `${METIS_DIR_*}` 旧宿主目录占位符都会被 `validate`/`pack` 拒绝。旧包不会自动改写，修改后请重新打包。

## 开发

```bash
go test ./...
go run ./cmd/metis --help
```

## 许可证

Apache-2.0，见 [LICENSE](LICENSE)。
