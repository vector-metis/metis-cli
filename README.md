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

## 开发

```bash
go test ./...
go run ./cmd/metis --help
```

## 许可证

Apache-2.0，见 [LICENSE](LICENSE)。
