<!--
 Copyright 2024 go-dataspace

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->

# RUN-DSP client usage

## Introduction

RUN-DSP comes with its own client subcommand. This command will execute miscellaneous tasks
on your RUN-DSP instance. These will vary from dataspace operations to administrative functions,
even though only dataspace operations are implemented at this time.

## Dependencies

The client depdends on a running RUN-DSP instance, with the control service enabled, as this
client does all its operations via or on this instance. This client will not do dataspace operations
itself, but offloads those to RUN-DSP.

## Client configuration

The client is configurable via a configuration file, environment variables, and command line flags.
Some flags (like json output) are flags only.

### Command line flags

You can see all command line options and subcommands by running:

```sh
./run-dsp client --help
```

All subcommands also accept the `--help` flag.

### Configuration file
We supply a [reference configuration file](../../conf/reference.toml)
that documents all all configuration keys, and their corresponding environment variables.
RUN-DSP will by default search for the configuration in `/etc/run-dsp/run-dsp.toml`, but you can
supply any with the `-c` flag.

## Usage examples

All client subcommands inherit the client connection configuration from the client subcommand.
This is normally configured in the configuration file or via environment variables, but can
also be given via flags. For example like this:

```sh
./run-dsp client \
    --address run-dsp.example.org:8080 \
    --authorization-metadata "Bearer abcde" \
    --ca-cert ./ca.pem \
    --client-cert ./client.pem \
    --client-cert-key ./client.key \
    getcatalog http://participant.example.org
```

In the following examples we assume that the client connection is set in the default configuration
file.

To get the catalog from `participant.example.org`:
```sh
./run-dsp client getcatalog http://participant.example.org
```

To get information about a single dataset with ID `5918b04a-1989-44f6-bbc0-a4df4d21ba67` from
`participant.example.org`:
```sh
./run-dsp client getdataset http://participant.example.org urn:uuid:5918b04a-1989-44f6-bbc0-a4df4d21ba67`
```
To download the dataset with ID `5918b04a-1989-44f6-bbc0-a4df4d21ba67` from `participant.example.org`:
```sh
./run-dsp client downloaddataset http://participant.example.org urn:uuid:5918b04a-1989-44f6-bbc0-a4df4d21ba67`
```
