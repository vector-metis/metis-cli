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

Apache-2.0，见仓库中的 `LICENSE`。
