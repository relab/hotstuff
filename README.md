# hotstuff

[![Go Reference](https://pkg.go.dev/badge/github.com/relab/hotstuff.svg)](https://pkg.go.dev/github.com/relab/hotstuff)
![Test](https://github.com/relab/hotstuff/workflows/Test/badge.svg)
![golangci-lint](https://github.com/relab/hotstuff/workflows/golangci-lint/badge.svg)
[![codecov](https://codecov.io/gh/relab/hotstuff/branch/master/graph/badge.svg?token=IYZ7WD6ZAH)](https://codecov.io/gh/relab/hotstuff)

`relab/hotstuff` is an implementation of the HotStuff protocol [1]. It uses the Gorums [2] RPC framework for sending messages between replicas.

## Running the examples

We have written an example client located in `cmd/hotstuffclient` and an example server located in `cmd/hotstuffserver`.
These can be compiled by running `make`.
They read a configuration file named `hotstuff.toml` from the working directory.
An example configuration that runs on localhost is included in the root of the project.
To generate public and private keys for the servers, run `cmd/hotstuffkeygen/hotstuffkeygen -p 'r*' -n 4 --hosts 127.0.0.1 --tls keys`.
To start four servers, run `scripts/run_servers.sh` with any desired options.
To start the client, run `cmd/hotstuffclient/hotstuffclient`.

## TODO

* Further benchmarking and testing
* Improving performance
  * Allow leaders to send command hashes instead of resending whole commands.
* Allow a replica to "catch up" by fetching missing nodes
* Add reconfiguration

## References

[1] M. Yin, D. Malkhi, M. K. Reiter, G. Golan Gueta, and I. Abraham, “HotStuff: BFT Consensus in the Lens of Blockchain,” Mar 2018.

[2] Tormod Erevik Lea, Leander Jehl, and Hein Meling. Towards New Abstractions for Implementing Quorum-based Systems. In 37th International Conference on Distributed Computing Systems (ICDCS), Jun 2017.
