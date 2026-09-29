// This program is the same hive CLI as cmd/hive. It stays at the module root
// so that `go install github.com/colonyops/hive@latest` continues to work.
package main

import "github.com/colonyops/hive/cmd/hive/cli"

func main() {
	cli.Main()
}
