# dockexec

Run Go tests inside a Docker image.

	go get mvdan.cc/dockexec
	go test -exec='dockexec postgres:12.1'

You can also use custom flags for `docker run`, as well as any test flags:

	go test -exec='dockexec [docker flags] image:tag' [test flags]

The goal is to easily test many packages with specific Docker images, without
having to write the boilerplate code yourself. All previous alternatives weren't
any good:

* Running `go test` inside `docker run` requires your Go version to be installed
  in the image.
* Running `go test -c` and running the test binary under `docker run` is
  tedious, error-prone, and doesn't scale to many packages.
