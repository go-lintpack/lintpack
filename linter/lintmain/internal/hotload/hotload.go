package hotload

import (
	"fmt"
	"plugin"

	"github.com/go-lintpack/lintpack"
)

// CheckersFromDylib loads checkers provided by a dynamic lybrary found under path.
//
// The returned info slice must be re-assigned to the original info slice,
// since there will be new entries there.
func CheckersFromDylib(infoList []*lintpack.CheckerInfo, path string) ([]*lintpack.CheckerInfo, error) {
	if path == "" {
		return infoList, nil // Nothing to do
	}
	checkersBefore := len(infoList)
	// Open plugin only for side-effects (init functions).
	_, err := plugin.Open(path)
	if err != nil {
		return infoList, err
	}
	maybeUpdatedList := lintpack.GetCheckersInfo()
	checkersAfter := len(maybeUpdatedList)
	if checkersBefore == checkersAfter {
		return infoList, fmt.Errorf("loaded plugin doesn't provide any lintpack-compatible checkers")
	}
	return maybeUpdatedList, nil
}
