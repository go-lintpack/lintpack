package lintmain

import (
	"fmt"
	"plugin"

	"github.com/go-lintpack/lintpack"
)

// checkersFromDylib loads checkers provided by a dynamic lybrary found under path.
func checkersFromDylib(path string) error {
	if path == "" {
		return nil // Nothing to do
	}
	checkersBefore := len(lintpack.GetCheckersInfo())
	// Open plugin only for side-effects (init functions).
	_, err := plugin.Open(path)
	if err != nil {
		return err
	}
	checkersAfter := len(lintpack.GetCheckersInfo())
	if checkersBefore == checkersAfter {
		return fmt.Errorf("loaded plugin doesn't provide any lintpack-compatible checkers")
	}
	return nil
}
