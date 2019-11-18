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
	byLine map[int][]*warning
}

type warning struct {
	matched bool
	text    string
}

func (w warning) String() string {
	return w.text
}

func newWarnings(r io.Reader) (*warnings, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read test file data: %w", err)
	}
	lines := strings.Split(string(b), "\n")

	ws := make(map[int][]*warning)
	var pending []*warning

	for i, l := range lines {
		if m := warningDirectiveRE.FindStringSubmatch(l); m != nil {
			pending = append(pending, &warning{text: m[1]})
		} else if len(pending) != 0 {
			line := i + 1
			ws[line] = append([]*warning{}, pending...)
			pending = pending[:0]
		}
	}
	return &warnings{byLine: ws}, nil
}

func (ws *warnings) find(line int, text string) *warning {
	for _, w := range ws.byLine[line] {
		if text == w.text {
			return w
		}
	}
	return nil
}

func (ws *warnings) checkUnmatched(t *testing.T, testFilename string) {
	for line, sl := range ws.byLine {
		for _, w := range sl {
			if !w.matched {
				t.Errorf("%s:%d: unmatched `%s`", testFilename, line, w)
			}
		}
	}
}
