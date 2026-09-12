// Command metis 提供开发者本地初始化、打包、校验和检查 MPK 的能力。
package main

import (
	"fmt"
	"os"

	cli "github.com/vector-metis/metis-cli"
)

var version = "dev"

func main() {
	if err := cli.NewCommand(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "metis:", err)
		os.Exit(1)
	}
}
