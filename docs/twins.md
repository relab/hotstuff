# Twins

Twins [1] is a strategy for testing byzantine consensus protocols for safety and liveness.
Twins emulates attacks from byzantine replicas by running two copies (twins) of the same replica.
Both twins share the same private key, and are thus indistinguishable from a single replica that is behaving oddly.

## Contents

- [Twins](#twins)
  - [Contents](#contents)
  - [Using the CLI](#using-the-cli)
    - [Generating Scenarios](#generating-scenarios)
    - [Executing Scenarios](#executing-scenarios)
  - [Implementation](#implementation)
    - [Partition Scenario Algorithm](#partition-scenario-algorithm)
  - [References](#references)

## Using the CLI

The HotStuff CLI includes support for generating and executing scenarios. This is done through the `twins` subcommand.
To see the list of available options, run `./hotstuff help twins`.

### Generating Scenarios

Scenarios can either be generated ahead of time and written to a file,
or they can be generated on the fly while executing them.
To generate scenarios ahead of time, use the command `./hotstuff twins generate`.
The generated scenarios will then be written in JSON format to one or multiple files.
The `--scenarios-per-file` flag controls how many scenarios will be written to each file,
and the `--output` flag controls where they will be written.
If the `--scenarios-per-file` flag is `0` (that is the default), the `--output` specifies the file to write to.
Otherwise, the `--output` flag specifies the directory in which multiple output files should be created.

The `--scenarios` flag controls how many scenarios will be generated.
If this flag is unspecified, the generator will generate all possible scenarios.
Note that this will likely result in several hundreds of megabytes of scenarios,
as the number of possible scenarios is quite large.
Hence it could be useful to specify the `--scenarios-per-file` flag if generating a large number of scenarios.

The `--replicas`, `--twins`, `--partitions`, and `--rounds` flags are the inputs for the scenario generator.
The `--replicas` flag specifies how many replicas to create.
The `--twins` flag specifies how many of the replicas should have a twin.
The `--partitions` flag specifies how many network partitions the replicas and twins should be divided into.
The `--rounds` flag specifies how many rounds or views to run.

The generator will always generate the same scenarios if given the same input parameters.
That is, unless the `--shuffle` flag is given, in which case the order of scenarios is randomized.
The `--seed` flag may be used to reproduce the same randomized order again.

The generator settings will be written to the head of each output file.

### Executing Scenarios

To execute scenarios, use the command `./hotstuff twins run`.
This command can either read scenarios from a JSON file, if the `--input` flag is given,
or generate scenarios on the fly,
in which case the flags described in the previous section may be used to configure the generator.

The consensus implementation is chosen by the `--consensus` flag.

The scenario executor also uses the `--output` and `--scenarios-per-file` flags,
but it only writes failed scenarios unless the `--log-all` flag is given.

To speed up execution, set the `--concurrency` flag to `0` to make use of all available CPUs.

## Implementation

Twins is implemented by two main components: a *scenario generator* and a *scenario executor*.
We have implemented the scenario executor by writing implementations of the `Configuration` and `Replica` interfaces from the `consensus` package.
These implementations allow us to control which messages are sent and which messages are dropped among replicas.
The scenario generator is responsible for generating scenarios for the scenario executor to execute.
A scenario describes the amount of replicas and twins to create, as well as the network partitions and leaders for each view.

The Twins paper [1] describes scenario generation as a three step process:

1. Generate all possible partitions, called partitions scenarios, of nodes (replicas and twins).
2. Assign each partition scenario to all possible leaders.
3. Generate all possible ways to arrange the leader, partition scenario pairs over a given number of views.

Unfortunately, the implementation of the first step is left out, so we had to come up with our own algorithm.
The algorithm we choose for implementing this step has a significant effect on how many scenarios our generator will generate.
Consider the following example:

{A, B} {A', C, D}

Here we have two partitions.
The first one contains the replicas A and B.
The second one contains a twin of replica A, called A', as well as replicas C and D.
We can expect this scenario to behave identically to the following scenario:

{A, D} {A', B, C}

This scenario is different from the first, because the replicas D and B have switched places,
but the behaviors of the scenarios are the same because both B and D follow the same protocol.
Now, consider the following scenario:

{A, A', B} {C, D}

The behavior of this scenario differs from the previous two because one of the twins have moved.
In general, we only want to generate the scenarios that differ in terms of the partition sizes and the positions of the twins.

### Partition Scenario Algorithm

The first part of generating the partition scenarios is to determine the possible sizes of partitions.
For n=5 nodes and k=3 partitions, we can have the following partition sizes:

| 0   | 1   | 2   |
| --- | --- | --- |
| 5   | 0   | 0   |
| 4   | 1   | 0   |
| 3   | 2   | 0   |
| 3   | 1   | 1   |
| 2   | 2   | 1   |

To generate this table, we can use the following algorithm:

```go
func partitionScenarios(n, k uint8) {
    state := make([]uint8, k)
    genPartitionScenarios(0, n, k, state)
}

func genPartitionScenarios(i, n, k uint8, state []uint8) {
    // i is the index of the partition to modify
    // n is the total number of nodes that we can assign

    // make a copy of the state
    state = append([]uint8(nil), state...)

    // first, attempt to assign all available nodes to the partition at index i.
    state[i] = n

    if i == 0 || state[i-1] >= n {
        // found a valid partition
        fmt.Println(state);
    }

    // find the next valid size for partition at index i
    var m = n - 1;
    if i > 0 {
        m = min(m, state[i-1])
    }

    // try all smaller sizes of partition at index i while handing the remaining nodes to the next partitions.
    if i + 1 < k {
        for ; m > 0; m-- {
            state[i] = m
            genPartitionScenarios(i+1, n-m, k, state)
        }
    }
}
```

The second part of generating the partition scenarios is to find all possible ways to assign the twins to the generated partition sizes.
We can find all possible ways to assign a single pair of twins by using the following algorithm:

```go
for i := 0; i < k; i++ {
    for j := i; j < k; k++ {
        fmt.Printf("(%d, %d)\n", i, j)
    }
}
```

Then, for *t* pairs of twins, we compute the cartesian product of this set with itself *t* times.

Finally, we generate all combinations of the partition sizes and the twin assignments that are valid and fill the
partitions with normal replicas.

## References

[1]: S. Bano, A. Sonnino, A. Chursin, D. Perelman, en D. Malkhi, “Twins: White-Glove Approach for BFT Testing”, 2020.
