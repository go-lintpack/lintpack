package linttest

import (
	"io/ioutil"
	"regexp"
	"strings"
	"testing"
)

var (
	warningDirectiveRE = regexp.MustCompile(`^\s*/\*! (.*) \*/`)
	commentRE          = regexp.MustCompile(`^\s*//`)
)

type goldenFile struct {
	warnings map[int][]*warning
}

type warning struct {
	matched bool
	text    string
}

func (w warning) String() string {
	return w.text
}

// TODO(Quasilyte): rename the function and a type.
// The're not related to a golden file.
func newGoldenFile(t *testing.T, filename string) *goldenFile {
	testData, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("can't find checker tests: %v", err)
	}
	lines := strings.Split(string(testData), "\n")

	warnings := make(map[int][]*warning)
	var pending []*warning

	for i, l := range lines {
		if m := warningDirectiveRE.FindStringSubmatch(l); m != nil {
			pending = append(pending, &warning{text: m[1]})
		} else if len(pending) != 0 {
			line := i + 1
			warnings[line] = append([]*warning{}, pending...)
			pending = pending[:0]
		}
	}
	return &goldenFile{warnings: warnings}
}

func (f *goldenFile) find(line int, text string) *warning {
	for _, y := range f.warnings[line] {
		if text == y.text {
			return y
		}
	}
	return nil
}

func (f *goldenFile) checkUnmatched(t *testing.T, testFilename string) {
	for line := range f.warnings {
		for _, w := range f.warnings[line] {
			if w.matched {
				continue
			}
			t.Errorf("%s:%d: unmatched `%s`", testFilename, line, w)
		}
	}
}
