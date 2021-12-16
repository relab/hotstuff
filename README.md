# Dat650 Project - Leader election schemes in Hotstuff

This project was extended from the `relab` framework, and focuses on new leader election schemes in hotstuff. https://pkg.go.dev/github.com/relab/hotstuff
Included in this project, we used the package `weightedrand`, https://pkg.go.dev/github.com/mroth/weightedrand@v0.4.1 

To run the project from `~/hotstuff`


First 
build: `go build -o ./hotstuff.exe ./cmd/hotstuff`

* Running Carousel algorithm: 
`./hotstuff.exe run --leader-rotation car`
* Running rep-based algorithm: 
`./hostuff.exe run --leader-rotation rep`

## Build Dependencies

* Go (at least version 1.16)
* Protobuf compiler
* Make

## Getting Started

* Install required go tools by running `make tools`.
* Compile command line utilities by running `make`.

## Running Experiments

See the [experimentation documentation](docs/experimentation.md) for details.

The `hotstuff` command line utility can be used to run experiments locally, or on remote servers using SSH.
For a list of options, run `./hotstuff help run`.

The `plot` command line utility can be used to create graphs from measurements.
Run `./plot --help` for a list of options.

## Modules

The following components are modular:

* Consensus
  * The "core" of the consensus protocol, which decides when a replica should vote for a proposal,
    and when a block should be committed.
  * 3 implementations:
    * `chainedhotstuff`: The three-phase pipelined HotStuff protocol presented in the HotStuff paper [1].
    * `fasthotstuff`: A two-chain version of HotStuff designed to prevent forking attacks [3].
    * `simplehotstuff`: A simplified version of chainedhotstuff [4].
* Crypto
  * Implements the cryptographic primitives used by HotStuff, namely quorum certificates.
  * 2 implementations:
    * `ecdsa`: A very simple implementation where quorum certificates are represented by arrays of ECDSA signatures.
    * `bls12`: An implementation of threshold signatures based on BLS12-381 aggregated signatures.
* Synchronizer
  * Implements a view-synchronization algorithm. It is responsible for synchronizing replicas to the same view number.
  * Current implementation based on [DiemBFT's RoundState](https://github.com/diem/diem/tree/main/consensus/src/liveness) [5].
* Blockchain
  * Implements storage for the block chain. Currently we have an in-memory cache of a fixed size.
* Leader rotation
  * Decides which replica should be the leader of a view.
  * Currently either a fixed leader or round-robin.
* Networking/Backend
  * Using [Gorums](https://github.com/relab/gorums) [2]

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
