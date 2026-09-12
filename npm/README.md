# npm 发布布局

`@vector-metis/cli` 是 npm 安装入口，平台二进制放在五个可选依赖包中。发布工作流先交叉编译 Go CLI，再把最终二进制复制到对应 `bin/`，随后在同一提交和版本中发布平台包与入口包。

入口脚本只根据 `process.platform`/`process.arch` 选择已经安装的可选依赖，不执行网络下载。缺少平台依赖或平台不受支持时返回明确错误。

用户只需要安装入口包：

```bash
npm install --global @vector-metis/cli
metis --version
```

入口包和五个可选平台包都会包含 README，npm 页面可直接查看安装和支持平台说明。
