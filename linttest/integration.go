package linttest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func defaultIntegrationTest() *IntegrationTest {
	return &IntegrationTest{
		Packages: []string{"."},
		Dir:      "./testdata/_integration",
	}
}

// Run executes integration tests.
func (cfg *IntegrationTest) Run(t *testing.T) {
	if err := exec.Command("lintpack", "version").Run(); err != nil {
		t.Skipf("lintpack is not available: %v", err)
	}

	absDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		t.Fatalf("can't get dir abs path: %v", err)
	}

	linter, err := cfg.buildLinter()
	if err != nil {
		t.Fatalf("build linter: %v", err)
	}

	files, err := ioutil.ReadDir(absDir)
	if err != nil {
		t.Fatalf("list test files: %v", err)
	}

	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		t.Run(f.Name(), func(t *testing.T) {
			wd := filepath.Join(absDir, f.Name())
			if err := os.Chdir(wd); err != nil {
				t.Fatalf("enter test dir: %v", err)
			}
			cfg.runTest(t, linter, wd)
		})
	}
}

func (cfg *IntegrationTest) runTest(t *testing.T, linter, gopath string) {
	data, err := ioutil.ReadFile("linttest.params")
	if err != nil {
		t.Fatalf("reading linter run params: %v", err)
	}

	// If several tests re-use a single golden file,
	// don't read it repeatedly, just re-use its contents.
	goldenDataCache := make(map[string]string)

	for i, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}

		// The format is:
		//	runParams ... "|" goldenFile
		parts := strings.Split(line, "|")
		runParams := strings.Split(strings.TrimSpace(parts[0]), " ")
		goldenFile := strings.TrimSpace(parts[1])

		// Read from a golden file or contents cache.
		var want string
		if data, ok := goldenDataCache[goldenFile]; ok {
			want = data
		} else {
			data, err := ioutil.ReadFile(goldenFile)
			if err != nil {
				t.Errorf("read golden file: %v", err)
			}
			want = string(data)
			goldenDataCache[goldenFile] = want
		}

		// Get the actual execution output.
		cmd := exec.Command(linter, runParams...)
		cmd.Env = append([]string{}, os.Environ()...) // Copy parent env
		cmd.Env = append(cmd.Env,
			// Override GOPATH.
			"GOPATH="+gopath,
			// Disable modules. See #62.
			"GO111MODULE=off")

		out, err := cmd.CombinedOutput()
		out = bytes.TrimSpace(out)
		var have string
		if err != nil {
			// Error is prepended to the beginning.
			have = err.Error() + "\n" + string(out)
		} else {
			have = string(out)
		}

		// To get line-by-line diff, split is required.
		wantLines := strings.Split(want, "\n")
		haveLines := strings.Split(have, "\n")
		if diff := cmp.Diff(wantLines, haveLines); diff != "" {
			t.Errorf("linttest.params:%d: output mismatch:\n%s", i+1, diff)
			t.Logf("linter output was: %s\n", have)
		}
	}
}

func (cfg *IntegrationTest) buildLinter() (string, error) {
	tmpDir := os.TempDir()
	linter := filepath.Join(tmpDir, "_lintpack_inttest_linter_")

	args := append([]string{"build", "-o", linter}, cfg.Packages...)
	out, err := exec.Command("lintpack", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, out)
	}

	return linter, nil
}
