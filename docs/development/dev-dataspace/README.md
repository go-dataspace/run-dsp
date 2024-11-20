# Simple development dataspace

## Introduction

A dataspace is a pretty complicated setup, it consists of multiple participants that request operations
from each other via the [dataspace protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol).

In order to test RUN-DSP, or to debug interactions between two participants, we've supplied this
development dataspace. It sets up two RUN-DSP instances and a provider. One of them is marked as
the `consumer` while the other is marked as the `provider`. What these roles are is described in
the above protocol specification.

## Prerequisites

This setup expects you have `docker-compose` installed. This setup is also tested in rootless
`podman`, as long as the environment variable `DOCKER_HOST` is set to the appropriate socket.

Client examples assume you have RUN-DSP installed and in your PATH, you can get a precompiled binary
[here](https://github.com/go-dataspace/run-dsp/releases/latest).

Alternatively, if you have go installed, you can also replace every instance of the `run-dsp` command
with `go run ./cmd/`, assuming you are in the root of this repository.

## Using the dev dataspace

To build a simple dataspace using the current state of your local git repository, just run the
following in the same directory as this file:

```bash
$ docker-compose up
```

This will start the following containers:

- `run-dsp-client`: The RUN-DSP instance that will act as a client.
- `run-dsp-provider`: The RUN-DSP instance that will act as a provider.
- `reference-provider`: The simple [reference provider](https://github.com/go-dataspace/reference-provider) that RUN-DSP will request dataset operations from.

Note, inside the docker-compose network, their names are also their hostnames.


And to see if everything works correctly, try to request the provider's catalog:

```bash
$ run-dsp -f ./dev-dataspace.conf client getcatalog http://run-dsp-provider:8080
```

This should display a catalog of two separate files, for more information on how to use the client,
you can view its documentation [here](../../usage/client.md).

## Development

To use this setup for development purposes, it will be good to know that the RUN-DSP instances are
all built from the local source base, so deleting and recreating the setup will incorporate any changes
you have made.

For debugging, both containers run their instances inside of a headless [delve](https://github.com/go-delve/delve)
debug session that you can connect to.

For the consumer instance:
```bash
$ dlv connect :14000
```

And for the provider instance:
```bash
$ dlv connect :24000
```

For convenience, we've also added two entries in the project's [launch.json](../../../.vscode/launch.json)
to allow [Visual Studio Code](https://code.visualstudio.com/) users to use the `dlv` integration
out of the box. Do note that these won't start up the development dataspace, this will have to be done by hand.
