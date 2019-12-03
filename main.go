// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

var flagSet = flag.NewFlagSet("dockexec", flag.ContinueOnError)

func init() { flagSet.Usage = usage }

func usage() {
	fmt.Fprintf(os.Stderr, `
Usage of dockexec:

	go test -exec='dockexec image:tag [args]'

Or, executing it directly:

	dockexec image:tag pkg.test [args]
`[1:])
	flagSet.PrintDefaults()
}

func main() { os.Exit(main1()) }

func main1() int {
	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return 2
	}
	args := flagSet.Args()

	if len(args) < 2 {
		flagSet.Usage()
		return 2
	}

	image := args[0]
	binary := args[1]

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	dockerArgs := []string{
		"run",

		// Delete the container when we're done.
		"--rm",

		// Set up the test binary as the entrypoint.
		fmt.Sprintf("--volume=%s:/init", binary),
		"--entrypoint=/init",

		// Set up the package directory as the workdir.
		fmt.Sprintf("--volume=%s:/pwd", wd),
		"--workdir=/pwd",

		// Use the specified image.
		image,
	}
	// Finally, pass the rest of the arguments to the test binary, such as
	// -test.timeout or -test.v flags.
	dockerArgs = append(dockerArgs, args[2:]...)

	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
