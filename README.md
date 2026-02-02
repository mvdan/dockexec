# dockexec

Run Go tests inside a Docker image.

	go install mvdan.cc/dockexec@latest
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

### Usage with `go tool`

dockexec can be used with the custom tool feature in go.mod:

```
go get -tool mvdan.cc/dockexec@latest
go test -exec='go tool dockexec postgres:12.1'
```

If the container's OS and architecture are not identical to the host,
this workaround is necessary to compile each part correctly:

```
GOOS=linux GOARCH=amd64 go test -exec 'env GOOS= GOARCH= go tool dockexec postgres:12.1'
```

The “outer” GOOS/GOARCH are for the test binary and have to match the container image.
The “inner” GOOS/GOARCH are for the host OS where dockexec runs.
They are set to empty to let them default to native.


### Caveats

* `go test` without package arguments runs tests with access to the current
  terminal. However, `go test -exec="dockexec $image"` will not, as `dockexec`
  cannot distinguish this mode from others like `go test -exec="dockexec $image"
  ./...`. If you want access to the terminal, supply the `-t` docker flag.

* Docker images are assumed to be unix-like at the moment, and only Linux images
  are tested. Other platforms like Windows-native images may be supported in the
  future.

* Beware that the Docker image may not have compatible C libraries, given the
  default of `CGO_ENABLED=1`. If you run into "no such file" exec errors,
  or glibc version mismatch errors, you should consider disabling cgo via
  `CGO_ENABLED=0` or a [fully static build](https://github.com/golang/go/issues/26492).

#### unable to start container process: error during container init: exec: "/init": is a directory: permission denied

This error happens when the test binary as created by `go test` is not available to the Docker daemon,
which then creates and mounts an empty directory.
It occurs when the Docker daemon is not running in the same environment as dockexec,
for example when [running Docker-in-Docker in Gitlab CI, where /tmp is not shared with it](https://docs.gitlab.com/ci/docker/using_docker_build/#known-issues-with-docker-in-docker).

As a workaround, you can set the environment variable `GOTMPDIR` to a directory that is shared.
In Gitlab CI, that is everything below the build directory:

```
mkdir -p tmp
GOTMPDIR=$PWD/tmp go test -exec 'dockexec …'
```

