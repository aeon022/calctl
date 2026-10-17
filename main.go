package main

import (
	"github.com/aeon022/calctl/cmd"
	"github.com/aeon022/missionctl-core/secure"
)

func main() {
	secure.Init("calctl") // owner-only files from the first byte
	cmd.Execute()
}
