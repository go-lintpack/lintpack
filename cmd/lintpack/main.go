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

var version = "v0.5.1"

var subCommands = []*cmdutil.SubCommand{
	{
		Main:  lintpackBuild,
		Name:  "build",
		Short: "build linter from made of lintpack-compatible packages",
		Examples: []string{
			"lintpack build -help",
			"lintpack build -o gocritic github.com/go-critic/checkers",
			"lintpack build -linter.version=v1.0.0 .",
		},
	},
	{
		Main:     lintpackVersion,
		Name:     "version",
		Short:    "print lintpack version",
		Examples: []string{"lintpack version"},
	},
}

func lintpackVersion() {
	fmt.Println(version)
}
