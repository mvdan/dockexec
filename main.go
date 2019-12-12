// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

var flagSet = flag.NewFlagSet("dockexec", flag.ContinueOnError)

func init() { flagSet.Usage = usage }

func usage() {
	fmt.Fprintf(os.Stderr, `
Usage of dockexec:

	go test -exec='dockexec [docker flags] image:tag' [test flags]

For example:

	go test -exec='dockexec postgres:12.1'
	go test -exec='dockexec postgres:12.1 -m 512m' -v -race

You can also run it directly, if you must:

	dockexec image:tag [docker flags] pkg.test [test flags]
`[1:])
	flagSet.PrintDefaults()
}

type usageErr string

func (u usageErr) Error() string { return string(u) }

type flagErr string

func (f flagErr) Error() string { return string(f) }

func main() { os.Exit(main1()) }

func main1() int {
	err := mainerr()
	if err == nil {
		return 0
	}
	switch err.(type) {
	case usageErr:
		fmt.Fprintln(os.Stderr, err)
		flagSet.Usage()
		return 2
	case flagErr:
		return 2
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func mainerr() error {
	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return flagErr(err.Error())
	}
	args := flagSet.Args()

	if len(args) < 2 {
		return usageErr("incorrect number of arguments")
	}
	image := args[0]
	args = args[1:]

	// The rest of the arguments are in the form of:
	//
	//   [docker flags] pkg.test [test flags]
	//
	// For now, parse this by looking for the first argument that doesn't
	// start with "-", and which contains ".test".If this isn't enough in
	// the long run, we can start parsing docker flags instead.
	var dockerFlags []string
	var binary string
	var testFlags []string
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") && strings.Contains(arg, ".test") {
			dockerFlags = args[:i]
			binary = arg
			testFlags = args[i+1:]
			break
		}
	}
	if binary == "" {
		return usageErr("could not find the test binary argument")
	}

	// First, start with our docker flags.
	allDockerArgs := []string{
		"run",

		// Delete the container when we're done.
		"--rm",

		// Set up the test binary as the entrypoint.
		fmt.Sprintf("--volume=%s:/init", binary),
		"--entrypoint=/init",

		// User uid and git for GOPATH and GOCACHE volume mappings
		fmt.Sprintf("--user=%v:%v", syscall.Getuid(), syscall.Getgid()),
	}

	// Add docker flags based on our context (module-aware, GOPATH or ad hoc mode)
	contextDockerFlags, err := buildDockerFlags()
	if err != nil {
		return err
	}
	allDockerArgs = append(allDockerArgs, contextDockerFlags...)

	// Then, add the user's docker flags.
	allDockerArgs = append(allDockerArgs, dockerFlags...)

	// Add "--" to stop all docker flags, plus the specified image.
	allDockerArgs = append(allDockerArgs, "--", image)

	// Finally, pass all the test arguments to the test binary, such as
	// -test.timeout or -test.v flags.
	allDockerArgs = append(allDockerArgs, testFlags...)

	cmd := exec.Command("docker", allDockerArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// buildDockerFlags returns a slice of docker flags based on the current
// context. We apply different logic based on whether we are in:
//
// * module-aware mode
// * GOPATH mode
// * ad hoc mode
//
// For all the scenarios below the test binary will be mounted as /init; GOPATH
// and GOCACHE are made available at canonical locations. The GOPATH mode
// description below describes how and where GOPATH is made available.
//
// Module-aware mode
// -----------------
// Assuming:
//
// * a module $m is rooted at $moddir
// * that the package $m/cmd/blah/ exists
// * a working directory of $moddir
// * that we run go test -exec='...' ./cmd/blah
//
// Then $moddir will be mounted as /start and the working directory will be
// /start/cmd/blah.
//
// GOPATH mode (to be implemented)
// -------------------------------
// Assuming:
//
// * GOPATH=/gp1:/gp2
// * that the package github.com/a/b/cmd/blah exists within /gp1/src
// * a working directory of /gp1/src/github.com/a/b
// * that we run go test -exec='...' ./cmd/blah
//
// Then /gp2 will be mounted as /gopath2, /gp1 will be mounted as /gopath1,
// GOPATH=/gopath1:/gopath2 will be set, and the working directory will be
// /gopath2/src/github.com/a/b
//
// TODO: implement GOPATH logic; for now we assume ad hoc mode logic.
//
// Ad hoc mode
// -----------
// Assuming:
//
// * a working directory of $dir
//
// Then $dir will be mounted as /start and the working directory will be
// /start
func buildDockerFlags() ([]string, error) {
	var res []string

	var env struct {
		GOCACHE string
		GOPATH  string
		GOMOD   string
	}
	envCmd := exec.Command("go", "env", "-json")
	out, err := envCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run %v: %v\n%s", strings.Join(envCmd.Args, " "), err, out)
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %v output: %v", strings.Join(envCmd.Args, " "), err)
	}

	// Normalise GOPATH elements for symlinks for the purposes of
	// GOPATH mode below
	var gp []string
	var dockerGp []string // the gopath elements for the container
	for i, v := range strings.Split(env.GOPATH, string(os.PathListSeparator)) {
		ev, err := filepath.EvalSymlinks(v)
		if err != nil {
			return nil, fmt.Errorf("failed to filepath.EvalSymlinks(%q)", v)
		}
		gp = append(gp, ev)
		dv := fmt.Sprintf("/gopath%v", i+1)
		res = append(res, fmt.Sprintf("--volume=%v:%v", ev, dv))
		dockerGp = append(dockerGp, dv)
	}
	res = append(res,
		"--env=GOPATH="+strings.Join(dockerGp, string(os.PathListSeparator)),
		fmt.Sprintf("--volume=%v:/gocache", env.GOCACHE),
		"--env=GOCACHE=/gocache",
	)

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %v", err)
	}

	if env.GOMOD != "" && env.GOMOD != os.DevNull {
		// we are in module-aware mode and have a main module
		var mod struct {
			Path string
			Dir  string
		}
		modCmd := exec.Command("go", "list", "-m", "-json")
		out, err := modCmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to run %v: %v\n%s", strings.Join(modCmd.Args, " "), err, out)
		}
		if err := json.Unmarshal(out, &mod); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %v output: %v", strings.Join(modCmd.Args, " "), err)
		}
		rel, err := filepath.Rel(mod.Dir, wd)
		if err != nil {
			return nil, fmt.Errorf("failed to determine %v relative to %v: %v", wd, mod.Dir, err)
		}
		res = append(res,
			fmt.Sprintf("--volume=%v:/start", mod.Dir),
			fmt.Sprintf("--workdir=%v", path.Join("/start", rel)), // TODO fix up when we properly support windows
		)
		return res, nil
	}

	// TODO: implement GOPATH logic; for now we assume ad hoc mode logic.

	res = append(res,
		fmt.Sprintf("--volume=%v:/start", wd),
		"--workdir=/start",
	)

	return res, nil
}
