# hotstuff

## TODO

* Benchmarking and further testing
* Allow a replica to "catch up" by fetching missing nodes

## Testing and debugging

* Unit tests `go test -v github.com/relab/hotstuff/pkg/hotstuff`
* Testing with `cmd/hotstuff`:
  * The `test.sh` script can be used to run 4 instances of hotstuff, based on `hotstuff_config.toml`
  * Make sure that the key files exist by running `generate_keys.sh`
  * If using round-robin: Create input files by running the splitfile program, e.g. `splitfile big.txt *.in`.
  * To save a log file, you can redirect the output of the script. For example `./test.sh > test.out 2>&1`
  * To debug the leader (replica 1), run the script with `./test.sh debug leader`.
  * To debug replica 2, run the script with `./test.sh debug replica`.

### Debugging with rr

* To run with `rr`, run the script with `./test.sh record > test.out 2>&1`.
* To debug a recording, run `dlv --headless --api-version=2 -l localhost:2345 --accept-multiclient replay rr/hotstuff-[A
  NUMBER]/ hotstuff` from within `cmd/hotstuff`. Then, connect with VSCode by hitting `f5` or clicking "connect to
  server" in the debugging menu. To rewind the program, it is required to connect a second delve client by running `dlv
  connect localhost:2345` and running the `rewind` command. (VScode does not support the rewind command yet.)
