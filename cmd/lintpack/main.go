package main

import (
	"fmt"
	"log"

	"github.com/go-lintpack/lintpack/internal/cmdutil"
)

func main() {
	log.SetFlags(0)
	cmdutil.DispatchCommand(subCommands)
}

var version = "v0.5.0"

var subCommands = []*cmdutil.SubCommand{
	{
		Main:  lintpackBuild,
		Name:  "build",
		Short: "build linter from made of lintpack-compatible packages",
	},
	{
		Main:  lintpackVersion,
		Name:  "version",
		Short: "print lintpack version",
	},
}

func lintpackVersion() {
	fmt.Println(version)
}
