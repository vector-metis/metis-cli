# @vector-metis/cli

智办应用开发 CLI，用于初始化应用工作目录、校验包、打包 MPK 和检查包元数据。

## 安装

```bash
npm install --global @vector-metis/cli
metis --version
```

入口包会按当前操作系统和 CPU 架构安装对应的原生平台包，不执行运行时网络下载。

## 命令

```text
metis init [目录]
metis pack <工作目录> --output <文件.mpk>
metis validate <工作目录或文件.mpk>
metis inspect <工作目录或文件.mpk>
```

## 应用目录与 Overlay

应用运行时由平台为每个安装分配独立的 scope。Manifest 中的沙箱 source 使用 `program`、`config`、`data`、`log` 和 `tmp` 逻辑目录；应用不需要知道 Worker 的宿主路径。

包内静态文件必须位于 `overlay/` 下，挂载时只能使用 `./overlay` 或 `./overlay/...`：

```yaml
services:
  web:
    mounts:
      - {source: ./overlay/config.yaml, target: /etc/app/config.yaml, read_only: true}
```

旧的 `./config.yaml`、路径越界、绝对路径和可写 overlay 挂载会被校验拒绝；CLI 不会自动改写旧包。

Apache-2.0，见仓库中的 `LICENSE`。
