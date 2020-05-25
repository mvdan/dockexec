# dockexec

Run Go tests inside a Docker image.

	go get mvdan.cc/dockexec
	go test -exec='dockexec postgres:12.1'

You can also use custom flags for `docker run`, as well as any test flags:

	go test -exec='dockexec [docker flags] image:tag' [test flags]

`go run` is also supported:

	go run -exec='dockexec postgres:12.1' .

The goal is to easily test many packages with specific Docker images, without
having to write the boilerplate code yourself. All previous alternatives weren't
any good:

* Running `go test` inside `docker run` requires your Go version to be installed
  in the image.
* Running `go test -c` and running the test binary under `docker run` is
  tedious, error-prone, and doesn't scale to many packages.

### Caveats

* `go test` without package arguments runs tests with access to the current
  terminal. However, `go test -exec="dockexec $image"` will not, as `dockexec`
  cannot distinguish this mode from others like `go test -exec="dockexec $image"
  ./...`. If you want access to the terminal, supply the `-t` docker flag.

* Docker images are assumed to be unix-like at the moment, and only Linux images
  are tested. Other platforms like Windows-native images may be supported in the
  future.
