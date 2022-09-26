// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rogpeppe/go-internal/gotooltest"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"dockexec": main1,
	}))
}

var update = flag.Bool("u", false, "update testscript output files")

func TestScript(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required to run dockexec tests")
	}

	t.Parallel()

	p := testscript.Params{
		Dir: filepath.Join("testdata", "script"),
		Setup: func(env *testscript.Env) error {
			bindir := filepath.Join(env.WorkDir, ".bin")
			if err := os.Mkdir(bindir, 0o777); err != nil {
				return err
			}
			binfile := filepath.Join(bindir, "dockexec")
			if runtime.GOOS == "windows" {
				binfile += ".exe"
			}
			if err := os.Symlink(os.Args[0], binfile); err != nil {
				return err
			}
			env.Vars = append(env.Vars, fmt.Sprintf("PATH=%s%c%s", bindir, filepath.ListSeparator, os.Getenv("PATH")))
			env.Vars = append(env.Vars, "TESTSCRIPT_COMMAND=dockexec")

			// In order to make go available inside containers where the guest
			// and host OS and arch match, we define HOST_GOROOT
			env.Vars = append(env.Vars, "HOST_GOROOT="+runtime.GOROOT())
			return nil
		},
		UpdateScripts:       *update,
		RequireExplicitExec: true,
	}
	if err := gotooltest.Setup(&p); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, p)
}
