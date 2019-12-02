// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

// go test -exec='dockexec image:tag' [test flags]

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func main() {
	flag.Parse()
	args := flag.Args()

	image := args[0]
	binary := args[1]

	if len(args) < 2 {
		flag.Usage()
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
		os.Exit(1)
	}
}
