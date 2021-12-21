# hotstuff

[![Go Reference](https://pkg.go.dev/badge/github.com/relab/consensus.svg)](https://pkg.go.dev/github.com/relab/hotstuff)
![Test](https://github.com/relab/hotstuff/workflows/Test/badge.svg)
![golangci-lint](https://github.com/relab/hotstuff/workflows/golangci-lint/badge.svg)
[![codecov](https://codecov.io/gh/relab/hotstuff/branch/master/graph/badge.svg?token=IYZ7WD6ZAH)](https://codecov.io/gh/relab/hotstuff)

`relab/hotstuff` is a framework for experimenting with HotStuff and similar consensus protocols.
It provides a set of modules and interfaces that make it easy to test different algorithms.
It also provides a tool for deploying and running experiments on multiple servers via SSH.

## Contents

- [hotstuff](#hotstuff)
  - [Contents](#contents)
  - [Build Dependencies](#build-dependencies)
  - [Getting Started](#getting-started)
    - [Linux and macOS](#linux-and-macos)
    - [Windows](#windows)
  - [Running Experiments](#running-experiments)
  - [Safety Testing with Twins](#safety-testing-with-twins)
  - [Modules](#modules)
  - [Consensus Interfaces](#consensus-interfaces)
  - [Crypto Interfaces](#crypto-interfaces)
  - [References](#references)

## Build Dependencies

- [Go](https://go.dev) (at least version 1.16)

If you modify any of the protobuf files, you will need the following to compile them:

- [Protobuf compiler](https://github.com/protocolbuffers/protobuf) (at least version 3.15)

- The gRPC and gorums plugins for protobuf.
  - Linux and macOS users can run `make tools` to install these.
  - Windows users must do the following:
    - Ensure dependencies are downloaded: `go mod download`
    - `go install github.com/relab/gorums/cmd/protoc-gen-gorums`
    - `go install google.golang.org/protobuf/cmd/protoc-gen-go`

## Getting Started

### Linux and macOS

- Compile command line utilities by running `make`.

### Windows

- Run `go build -o ./hotstuff.exe ./cmd/hotstuff`
- Run `go build -o ./plot.exe ./cmd/plot`

**NOTE**: you should use `./hotstuff.exe` instead of `./hotstuff` when running commands.

**NOTE**: these commands will not recompile the protobuf files if they have changed.
You could try to run `make`, but it might not work on Windows. Instead, you can do it manually like this:

1. Get the path to the `gorums` module:

  ```text
  go list -m -f '{{.Dir}}' github.com/relab/gorums
  ```

2. Run `protoc` (replace `PATH_TO_GORUMS` with the output from the previous step):

  ```text
  protoc -I=PATH_TO_GORUMS:. --go_out=paths=source_relative:. --gorums_out=paths=source_relative:. PATH_TO_CHANGED_PROTO_FILE
  ```

## Running Experiments

See the [experimentation documentation](docs/experimentation.md) for details.

The `hotstuff` command line utility can be used to run experiments locally, or on remote servers using SSH.
For a list of options, run `./hotstuff help run`.

The `plot` command line utility can be used to create graphs from measurements.
Run `./plot --help` for a list of options.

## Safety Testing with Twins

We have implemented the Twins strategy [6] for testing the safety of the consensus implementations.
See the [twins documentation](docs/twins.md) for details.

## Modules

The following components are modular:

- Consensus
  - The "core" of the consensus protocol, which decides when a replica should vote for a proposal,
    and when a block should be committed.
  - 3 implementations:
    - `chainedhotstuff`: The three-phase pipelined HotStuff protocol presented in the HotStuff paper [1].
    - `fasthotstuff`: A two-chain version of HotStuff designed to prevent forking attacks [3].
    - `simplehotstuff`: A simplified version of chainedhotstuff [4].
- Crypto
  - Implements the cryptographic primitives used by HotStuff, namely quorum certificates.
  - 2 implementations:
    - `ecdsa`: A very simple implementation where quorum certificates are represented by arrays of ECDSA signatures.
    - `bls12`: An implementation of threshold signatures based on BLS12-381 aggregated signatures.
- Synchronizer
  - Implements a view-synchronization algorithm. It is responsible for synchronizing replicas to the same view number.
  - Current implementation based on [DiemBFT's RoundState](https://github.com/diem/diem/tree/main/consensus/src/liveness) [5].
- Blockchain
  - Implements storage for the block chain. Currently we have an in-memory cache of a fixed size.
- Leader rotation
  - Decides which replica should be the leader of a view.
  - Currently either a fixed leader or round-robin.
- Networking/Backend
  - Using [Gorums](https://github.com/relab/gorums) [2]

These modules are designed to be interchangeable and simple to implement.
It is also possible to add custom modules that can interact with the modules we listed above.

## Consensus Interfaces

There are two ways to implement a new consensus protocol, depending on the requirements of the new protocol.
The `Consensus` module interface is the most general, but also the most difficult to implement.
That is because it is oriented towards the other modules, and the other modules rely on it to perform certain operations,
such as verifying proposals, interacting with the `BlockChain`, `Acceptor`, `Executor`, and `Synchronizer` modules,
and so on. That is why we provide a simplified `Rules` interface and the optional `ProposeRuler` interface.
These interfaces are only required to implement the core rules of the consensus protocol, namely deciding whether or not
to vote for a proposal, whether or not it is safe to commit a block, and optionally how to create new blocks.
These interfaces do not require any interaction with other modules, as that is taken care of by a
[default implementation](consensus/consensus.go) of the `Consensus` interface.

The `Consensus` and `Rules` interfaces can also be used to override the behavior of other consensus implementations.
The `consensus/byzantine` package contains some examples of byzantine behaviors that can be implemented by wrapping
implementations of these interfaces.

## Crypto Interfaces

In a similar way to the `Consensus` interface, the `Crypto` interface also introduces a simplified, lower-level interface
that should be used to implement new crypto algorithms.
The `Crypto` interface itself provides methods for working with the high-level structures such as quorum certificates.
However, to make it easy to implement new crypto algorithms, the `CryptoImpl` interface can be used instead.
This interface deals with signatures and threshold signatures directly, which are used by the `Crypto` interface
to implement quorum certificates.

## References

[1] M. Yin, D. Malkhi, M. K. Reiter, G. Golan Gueta, and I. Abraham, “HotStuff: BFT Consensus in the Lens of Blockchain,” Mar 2018.

[2] Tormod Erevik Lea, Leander Jehl, and Hein Meling. Towards New Abstractions for Implementing Quorum-based Systems. In 37th International Conference on Distributed Computing Systems (ICDCS), Jun 2017.

[3] Mohammad M. Jalalzai, Jianyu Niu, Chen Feng, Fangyu Gai. Fast-HotStuff: A Fast and Resilient HotStuff Protocol, Oct 2020.

[4] Leander Jehl. Formal Verification of HotStuff, Jun 2021.

[5] Baudet, Mathieu, et al. "State machine replication in the libra blockchain." The Libra Assn., Tech. Rep (2019).

[6]: S. Bano, A. Sonnino, A. Chursin, D. Perelman, en D. Malkhi, “Twins: White-Glove Approach for BFT Testing”, 2020.
