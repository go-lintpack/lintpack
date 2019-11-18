package linttest

import (
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
	"testing"
)

var (
	warningDirectiveRE = regexp.MustCompile(`^\s*/\*! (.*) \*/`)
	commentRE          = regexp.MustCompile(`^\s*//`)
)

type warnings struct {
	byLine  map[int][]string
	matched map[*string]struct{}
}

func newWarnings(r io.Reader) (*warnings, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read test file data: %w", err)
	}
	lines := strings.Split(string(b), "\n")

	ws := make(map[int][]string)
	var pending []string

	for i, l := range lines {
		if m := warningDirectiveRE.FindStringSubmatch(l); m != nil {
			pending = append(pending, m[1])
		} else if len(pending) != 0 {
			line := i + 1
			ws[line] = pending
			pending = nil
		}
	}
	return &warnings{
		byLine:  ws,
		matched: make(map[*string]struct{}),
	}, nil
}

func (ws *warnings) find(line int, text string) *string {
	for _, w := range ws.byLine[line] {
		if text == w {
			return &w
		}
	}
	return nil
}

func (ws *warnings) checkUnmatched(t *testing.T, testFilename string) {
	for line, sl := range ws.byLine {
		for i, w := range sl {
			if _, ok := ws.matched[&sl[i]]; !ok {
				t.Errorf("%s:%d: unmatched `%s`", testFilename, line, w)
			}
		}
	}
}
