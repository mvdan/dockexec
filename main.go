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
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

var (
	flagSet  = flag.NewFlagSet("dockexec", flag.ExitOnError)
	fCompose = flagSet.Bool("compose", false, "use 'docker compose' instead of docker to run the test binary")
)

func init() { flagSet.Usage = usage }

func usage() {
	fmt.Fprint(os.Stderr, `
Usage of dockexec:

	go test -exec='dockexec [docker run flags] image:tag' [test flags]

Or, to use 'docker compose' instead of docker to run the test binary:

	go test -exec='dockexec -compose [docker run flags] service' [test flags]

For example:

	go test -exec='dockexec postgres:12.1'
	go test -exec='dockexec postgres:12.1 -m 512m' -v -race

You can also run it directly, if you must:

	dockexec image:tag [docker flags] pkg.test [test flags]

Flags:
`[1:])
	flagSet.PrintDefaults()
}

func main() {
	flagSet.Parse(os.Args[1:])
	args := flagSet.Args()

	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "incorrect number of arguments")
		flagSet.Usage()
		os.Exit(2)
	}
	image := args[0]
	args = args[1:]

	// The rest of the arguments are in the form of:
	//
	//   [docker flags] pkg.test [test flags]
	//
	// For now, parse this by looking for the first argument that doesn't start
	// with "-", and which looks like a Go binary (remembering that main
	// packages at the module root might contain dots, e.g. foo.com). If this
	// isn't enough in the long run, we can start parsing docker flags instead.
	//
	// As of today, the binary can look like (possibly with an ".exe" suffix):
	//
	//     go test: [...]/go-build[...]/b[...]/${pkg}.test
	//     go run:  [...]/go-build[...]/b[...]/exe/bar
	var dockerFlags []string
	var binary string
	var testFlags []string
	rxBinary := regexp.MustCompile(`\.test(\.exe)?$|/exe/[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)?(\.exe)?$`)
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") && rxBinary.MatchString(arg) {
			dockerFlags = args[:i]
			binary = arg
			testFlags = args[i+1:]
			break
		}
	}
	if binary == "" {
		fmt.Fprintln(os.Stderr, "could not find the test binary argument")
		flagSet.Usage()
		os.Exit(2)
	}

	tempHome, err := os.MkdirTemp("", "dockexec-home")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() {
		if err := os.RemoveAll(tempHome); err != nil {
			fmt.Println(err) // warn the user
		}
	}()
	realHome, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// First, start with our docker flags.
	var allDockerArgs []string
	if *fCompose {
		allDockerArgs = append(allDockerArgs, "compose")
	}
	allDockerArgs = append(allDockerArgs,
		"run",

		// Delete the container when we're done.
		"--rm",

		// Set up the test binary as the entrypoint.
		fmt.Sprintf("--volume=%s:/init", binary),
		"--entrypoint=/init",

		// User uid and gid so mounting HOME, GOCACHE, etc just works.
		fmt.Sprintf("--user=%v:%v", syscall.Getuid(), syscall.Getgid()),

		// Mount host files so the container can know what UID and GID stand for.
		// Note that we don't mount /etc/shadow, as we shouldn't need passwords.
		"--volume=/etc/passwd:/etc/passwd:ro",
		"--volume=/etc/group:/etc/group:ro",

		// Also mount a temporary empty directory as the user's home.
		// We don't want to mount the host's real home, to prevent harm.
		// We still need $HOME to exist as a directory, for completeness.
		fmt.Sprintf("--volume=%s:%s", tempHome, realHome),
	)

	// Ensure both systems agree on where $HOME is.
	// We don't want discrepancies because of /etc/passwd or cgo.
	// Note that this is HOME on most systems except Windows.
	if runtime.GOOS != "windows" {
		allDockerArgs = append(allDockerArgs, "-e", "HOME="+realHome)
	} else {
		allDockerArgs = append(allDockerArgs, "-e", "USERPROFILE="+realHome)
	}

	// Add docker flags based on our context (module-aware or ad hoc mode)
	contextDockerFlags, err := buildDockerFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	allDockerArgs = append(allDockerArgs, contextDockerFlags...)

	// Then, add the user's docker flags.
	allDockerArgs = append(allDockerArgs, dockerFlags...)

	// Add "--" to stop all docker flags if we are not in compose mode.
	allDockerArgs = append(allDockerArgs, "--")

	// Add the docker image/service name
	allDockerArgs = append(allDockerArgs, image)

	// Finally, pass all the test arguments to the test binary, such as
	// -test.timeout or -test.v flags.
	allDockerArgs = append(allDockerArgs, testFlags...)

	cmd := exec.Command("docker", allDockerArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// buildDockerFlags returns a slice of docker flags based on the current
// context. We apply different logic based on whether we are in:
//
// * module-aware mode
// * ad hoc mode
//
// For all the scenarios below the test binary will be mounted as /init;
// GOMODCACHE and GOCACHE are made available at canonical locations.
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
		GOMODCACHE string
		GOCACHE    string
		GOMOD      string
	}
	envCmd := exec.Command("go", "env", "-json")
	out, err := envCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run %v: %v\n%s", strings.Join(envCmd.Args, " "), err, out)
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %v output: %v", strings.Join(envCmd.Args, " "), err)
	}

	res = append(res,
		fmt.Sprintf("--volume=%v:/gomodcache", env.GOMODCACHE),
		"--env=GOMODCACHE=/gomodcache",
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

	// Ad-hoc mode.
	res = append(res,
		fmt.Sprintf("--volume=%v:/start", wd),
		"--workdir=/start",
	)

	return res, nil
}
