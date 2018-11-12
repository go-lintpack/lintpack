package lintmain

import (
	"log"

	"github.com/go-lintpack/lintpack/internal/cmdutil"
	"github.com/go-lintpack/lintpack/linter/lintmain/internal/check"
)

// Config is used to parametrize the linter.
type Config struct {
	Version string
}

var config *Config

// Run executes corresponding main after sub-command resolving.
// Does not return.
func Run(cfg Config) {
	config = &cfg // TODO(quasilyte): don't use global var for this
	log.SetFlags(0)
	cmdutil.DispatchCommand(subCommands)
}

var subCommands = []*cmdutil.SubCommand{
	{
		Main:  check.Main,
		Name:  "check",
		Short: "run linter over specified targets",
	},
	{
		Main:  printVersion,
		Name:  "version",
		Short: "print linter version",
	},
}

func printVersion() {
	log.Println(config.Version)
}
