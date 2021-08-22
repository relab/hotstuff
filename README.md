# hotstuff

[![Go Reference](https://pkg.go.dev/badge/github.com/relab/consensus.svg)](https://pkg.go.dev/github.com/relab/hotstuff)
![Test](https://github.com/relab/hotstuff/workflows/Test/badge.svg)
![golangci-lint](https://github.com/relab/hotstuff/workflows/golangci-lint/badge.svg)
[![codecov](https://codecov.io/gh/relab/hotstuff/branch/master/graph/badge.svg?token=IYZ7WD6ZAH)](https://codecov.io/gh/relab/hotstuff)

`relab/hotstuff` is a framework for experimenting with HotStuff and similar consensus protocols.
It provides a set of modules and interfaces that make it easy to test different algorithms.
It also provides a tool for deploying and running experiments on multiple servers via SSH.

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
* Blockchain
  * Implements storage for the block chain. Currently we have an in-memory cache of a fixed size.
* Leader rotation
  * Decides which replica should be the leader of a view.
  * Currently either a fixed leader or round-robin.
* Networking/Backend
  * Using [Gorums](https://github.com/relab/gorums) [2]

These modules are designed to be interchangeable and simple to implement.
It is also possible to add custom modules that can interact with the modules we listed above.

## Running Experiments

The `hotstuff` command line utility can be used to run experiments locally, or on remote servers using SSH.
For a list of options, run `hotstuff help run`.

## References

[1] M. Yin, D. Malkhi, M. K. Reiter, G. Golan Gueta, and I. Abraham, “HotStuff: BFT Consensus in the Lens of Blockchain,” Mar 2018.

[2] Tormod Erevik Lea, Leander Jehl, and Hein Meling. Towards New Abstractions for Implementing Quorum-based Systems. In 37th International Conference on Distributed Computing Systems (ICDCS), Jun 2017.

[3] Mohammad M. Jalalzai, Jianyu Niu, Chen Feng, Fangyu Gai. Fast-HotStuff: A Fast and Resilient HotStuff Protocol, Oct 2020.

[4] Leander Jehl. Formal Verification of HotStuff, Jun 2021.
