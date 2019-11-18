package linttest

import (
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
)

var (
	warningDirectiveRE = regexp.MustCompile(`^\s*/\*! (.*) \*/`)
	commentRE          = regexp.MustCompile(`^\s*//`)
)

type warnings map[int][]string

func newWarnings(r io.Reader) (warnings, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read test file data: %w", err)
	}
	lines := strings.Split(string(b), "\n")

	ws := make(warnings)
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

	return ws, nil
}

func (ws warnings) find(line int, text string) *string {
	for _, w := range ws[line] {
		if text == w {
			return &w
		}
	}
	return nil
}
