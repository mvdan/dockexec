// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// First, start with our docker flags.
	allDockerArgs := []string{
		"run",

		// Delete the container when we're done.
		"--rm",

		// Set up the test binary as the entrypoint.
		fmt.Sprintf("--volume=%s:/init", binary),
		"--entrypoint=/init",

		// Set up the package directory as the workdir.
		fmt.Sprintf("--volume=%s:/pwd", wd),
		"--workdir=/pwd",
	}

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
