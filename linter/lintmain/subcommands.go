package lintmain

import (
	"log"

	"github.com/go-lintpack/lintpack/linter/lintmain/internal/check"
)

// subCommands describes all supported sub-commands as well
// as their metadata required to run them and print useful help messages.
var subCommands = []*subCommand{
	{
		main:  check.Main,
		name:  "check",
		short: "run linter over specified targets",
	},
	{
		main:  printVersion,
		name:  "version",
		short: "print linter version",
	},
}

// subCommand is an implementation of a linter sub-command.
type subCommand struct {
	// main is command entry point.
	main func()

	// name is sub-command name used to execute it.
	name string

	// short describes command in one line of text.
	short string
}

// findSubCommand looks up subCommand by its name.
// Returns nil if requested command not found.
func findSubCommand(name string) *subCommand {
	for _, cmd := range subCommands {
		if cmd.name == name {
			return cmd
		}
	}
	return nil
}

func printVersion() {
	log.Println(config.Version)
}
