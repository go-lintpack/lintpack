package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"text/template"

	"github.com/go-lintpack/lintpack/linter/lintmain"
	"golang.org/x/tools/go/packages"
)

func main() {
	log.SetFlags(0)

	var p packer

	defer func() {
		if p.main != nil {
			_ = os.Remove(p.main.Name())
		}
	}()

	var steps = []struct {
		name string
		fn   func() error
	}{
		{"parse args", p.parseArgs},
		{"resolve packages", p.resolvePackages},
		{"create main file", p.createMainFile},
		{"build linter", p.buildLinter},
	}

	for _, step := range steps {
		if err := step.fn(); err != nil {
			log.Fatalf("%s: %v", step.name, err)
		}
	}
}

type packer struct {
	// Exported fields are used inside text template.

	flags struct {
		args []string
	}

	Packages []string
	Config   lintmain.Config

	outputFilename string

	main *os.File
}

func (p *packer) parseArgs() error {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: lintpack [flags] packages...\n")
		fmt.Fprintf(out, "package can be specified by a relative path, like `.` or `./...`\n")
		out.Write([]byte("\n"))
		flag.PrintDefaults()
	}

	flag.StringVar(&p.Config.Version, "linterVersion", "0.0.1",
		`value that will be printed by the linter "version" command`)
	flag.StringVar(&p.outputFilename, "o", "linter",
		`produced binary filename`)

	flag.Parse()

	p.flags.args = flag.Args()

	if len(p.flags.args) == 0 {
		return errors.New("not enough arguments: expected non-empty package list")
	}

	return nil
}

func (p *packer) resolvePackages() error {
	cfg := &packages.Config{Mode: packages.LoadFiles}
	pkgs, err := packages.Load(cfg, p.flags.args...)
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		p.Packages = append(p.Packages, pkg.PkgPath)
	}

	return nil
}

func (p *packer) createMainFile() error {
	mainFile, err := ioutil.TempFile("", "linter*.go")
	if err != nil {
		return fmt.Errorf("create tmp file: %v", err)
	}
	p.main = mainFile

	mainTmpl := template.Must(template.New("main").Parse(`
		package main
		import (
			"github.com/go-lintpack/lintpack/linter/lintmain"
			{{range .Packages}}
			_ "{{.}}" // Imported for lintpack.AddChecker calls
			{{end}}
		)
		func main() {
			cfg := {{printf "%#v" .Config}}
			lintmain.Run(cfg)
		}`))
	if err := mainTmpl.Execute(mainFile, &p); err != nil {
		return fmt.Errorf("execute template: %v", err)
	}

	return nil
}

func (p *packer) buildLinter() error {
	command := exec.Command("go", "build",
		"-o", p.outputFilename,
		p.main.Name())
	out, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %v:\n%s", err, out)
	}
	return nil
}
