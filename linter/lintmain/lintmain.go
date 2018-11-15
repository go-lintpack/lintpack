package lintmain

import (
	"log"
	"strings"

	"github.com/go-lintpack/lintpack/internal/cmdutil"
	"github.com/go-lintpack/lintpack/linter/lintmain/internal/check"
)

// Config is used to parametrize the linter.
type Config struct {
	Version string
	Name    string
}

var config *Config

// Run executes corresponding main after sub-command resolving.
// Does not return.
func Run(cfg Config) {
	config = &cfg // TODO(quasilyte): don't use global var for this
	log.SetFlags(0)

	// makeExample replaces all ${linter} placeholders to a bound linter name.
	makeExamples := func(examples ...string) []string {
		from := "${linter} "
		to := cfg.Name + " "
		for i := range examples {
			examples[i] = strings.Replace(examples[i], from, to, 1)
		}
		return examples
	}

	subCommands := []*cmdutil.SubCommand{
		{
			Main:  check.Main,
			Name:  "check",
			Short: "run linter over specified targets",
			Examples: makeExamples(
				"${linter} check -help",
				"${linter} check -disableTags=none strings bytes",
				"${linter} check -enableTags=diagnostic ./...",
			),
		},
		{
			Main:     printVersion,
			Name:     "version",
			Short:    "print linter version",
			Examples: makeExamples("${linter} version"),
		},
	}

	cmdutil.DispatchCommand(subCommands)
}

func printVersion() {
	log.Println(config.Version)
}
