package lintmain

import (
	"fmt"
	"log"
	"os"
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

	argv := os.Args
	if len(argv) < 2 {
		terminate("not enough arguments, expected sub-command name", printUsage)
	}

	subIdx := 1 // [0] is program name
	sub := os.Args[subIdx]
	// Erase sub-command argument (index=1) to make it invisible for
	// sub commands themselves.
	os.Args = append(os.Args[:subIdx], os.Args[subIdx+1:]...)

	// Choose and run sub-command main.
	cmd := findSubCommand(sub)
	if cmd == nil {
		terminate("unknown sub-command: "+sub, printSupportedSubs)
	}

	// The called function may exit with non-zero status.
	// No code should follow this call.
	cmd.main()
}

// terminate prints error specified by reason, runs optional printHelp
// function and then exists with non-zero status.
func terminate(reason string, printHelp func()) {
	stderrPrintf("error: %s\n", reason)
	if printHelp != nil {
		stderrPrintf("\n")
		printHelp()
	}
	os.Exit(1)
}

func printUsage() {
	// TODO: implement me. For now, print supported commands.
	printSupportedSubs()
}

func printSupportedSubs() {
	stderrPrintf("Supported sub-commands:\n")
	for _, cmd := range subCommands {
		stderrPrintf("\t%s - %s\n", cmd.name, cmd.short)
	}
}

// stderrPrintf writes formatted message to stderr and checks for error
// making "not annoying at all" linters happy.
func stderrPrintf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(os.Stderr, format, args...)
	if err != nil {
		panic(fmt.Sprintf("stderr write error: %v", err))
	}
}
