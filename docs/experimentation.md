# Experimentation

This document explains how to use the `hotstuff` cli tool to deploy and run experiments locally and on remote hosts.

## Contents

- [Experimentation](#experimentation)
  - [Contents](#contents)
  - [Metrics collection](#metrics-collection)
    - [Create a Protobuf message describing a measurement](#create-a-protobuf-message-describing-a-measurement)
    - [Emitting events from other modules](#emitting-events-from-other-modules)
    - [Create a module for your metric](#create-a-module-for-your-metric)
    - [Register your metric in the metrics registry](#register-your-metric-in-the-metrics-registry)
  - [Running experiments locally](#running-experiments-locally)
    - [Basic flags](#basic-flags)
    - [Client flags](#client-flags)
    - [Replica flags](#replica-flags)
    - [Module flags](#module-flags)
    - [Metrics flags](#metrics-flags)
    - [Performance monitoring flags](#performance-monitoring-flags)
  - [Running experiments on remote hosts](#running-experiments-on-remote-hosts)
    - [Manual assignment of clients and replicas](#manual-assignment-of-clients-and-replicas)
  - [Plotting measurements](#plotting-measurements)

## Metrics collection

This project includes a few useful metrics, such as (client) latency and throughput.
The module and event system is designed to make it easy to add additional metrics.
The following describes the steps needed to create a new metric and use it to perform experiments:

### Create a Protobuf message describing a measurement

These messages can for example be added to the `types.proto` file in the `metrics/types` package,
or any other package, as long as it is imported into the program.
The message should include a field named `Event` with the type `types.Event` from the proto file mentioned earlier.
Embedding this message will make your measurement compatible with some parts of the `plotting` package,
which we will discuss later. An example message is shown below:

```protobuf
message LatencyMeasurement {
  Event Event = 1;
  double Latency = 2;
  double Variance = 3;
  uint64 Count = 4;
}
```

### Emitting events from other modules

When creating a new metric, you will likely find it necessary to emit a new event type from some other module.
The preferred way to do this is to define the event type in that module's package, and then add that event to the
event loop when and where it is relevant to do so. For example, the throughput measurement receives `CommitEvent`
events from the `Executor` module. The event type should not be defined in the metric's package,
as that would require the other module to import the metrics module.

```go
srv.mods.EventLoop().AddEvent(consensus.CommitEvent{Commands: len(batch.GetCommands())}
```

**NOTE:** In earlier versions, we used a separate event loop for metrics.
This is no longer the case, and though the method `MetricsEventLoop()` remains,
it now returns the same event loop as the `EventLoop()` method.

### Create a module for your metric

To be able to interact with the event loop and other modules, you must implement either the `modules.Module`
or `consensus.Module` interfaces. The former should be preferred unless you need to access any of the consensus
modules directly. In the `InitModule` or `InitConsensusModule` functions you should add an observer or handler for
the events that you want to receive.

You should also add an observer for the `types.TickEvent` type on the `EventLoop`.
This event is sent at a configured interval such that each metric can periodically log its measurement.
The example below shows a complete initialization function for the throughput metric module.

```go
// InitModule implements the modules.Module interface
func (t *Throughput) InitModule(mods *modules.Modules) {
    // store reference to modules object if it needs to be used later
    t.mods = mods

    // register handlers/observers for relevant events
    t.mods.EventLoop().RegisterHandler(consensus.CommitEvent{}, func(event interface{}) {
        // The event loop will only call the handler with events of the specified type, so this assertion is safe.
        commitEvent := event.(consensus.CommitEvent)
        t.recordCommit(commitEvent.Commands)
    })

    // register observer for tick events
    t.mods.EventLoop().RegisterObserver(types.TickEvent{}, func(event interface{}) {
        t.tick(event.(types.TickEvent))
    })

    t.mods.Logger().Info("Throughput metric enabled")
}
```

### Register your metric in the metrics registry

The `metrics` package includes its own registry where you can register your metrics.
Registering your metric will make it available in the cli, as long as it is imported either directly or indirectly by
the cli. Registering the metric should be done in an `init()` function in your metric's package, and should call either
`RegisterReplicaMetric` or `RegisterClientMetric` (or both), depending on whether it collects metrics from clients or
replicas. The arguments to these functions are the name of the metric, and a constructor function that returns a new
instance of the metric. The name provided to this function can then be passed to the `--metrics` parameter of the cli,
in order to be enabled.

```go
func init() {
    RegisterReplicaMetric("throughput", func() interface{} {
        return &Throughput{}
    })
}
```

## Running experiments locally

First, compile the cli by running `make` from the project's root directory. If you get any errors, make sure that you
have go version 1.16 or later, as well as a protobuf compiler installed. You might also need to run `make tools` to
install some additional tools. The next sections will assume that you have done this, and that you are running the cli
from the project's root directory.

Running experiments is done by using the `./hotstuff run` command.
By default, this command will start up 4 replicas and a client locally, and run for 10 seconds with the default set of
modules and options. A large number of configuration options allows the experiments to be customized.
The options can be set either via command-line flags, or by writing a configuration file.
We will now explain the most important flags for the `run` command.
Each flag corresponds to a field in the config file with the same name (without the leading dashes).
For an example config file, see the [section about remote experiments](#manual-assignment-of-clients-and-replicas).
For the full list of flags, run `./hotstuff help run`.

### Basic flags

- `--config` the path to the config file to read.
- `--log-level` sets the lowest logging level. Valid values are `debug`, `info`, `warn`, and `error`. Defaults to `info`.
- `--log-pkgs` sets the logging level on a per-package basis, overriding the value set by `--log-level`. For example,
  passing `--log-pkgs="consensus:info,synchronizer:warn"` will set the log level for the `consensus` package to `info`
  and the `synchronizer` package to `warn`. Note that using this option will increase the overhead of logging somewhat.
- `--connect-timeout` how long to wait for the initial connection attempt.
- `--clients` the number of clients to run.
- `--replicas` the number of replicas to run.
- `--duration` the duration of the experiment. The argument should be given as a string in
  [Go's duration string format](https://pkg.go.dev/time#ParseDuration).

### Client flags

- `--payload-size` the size of each client command's payload (in bytes).
- `--max-concurrent` the maximum number of concurrent or in-flight commands that a client can have at a time.
  Note that if this number is less than $\frac{3 \cdot batch size}{clients}$, the replicas will not have enough commands
  to make progress.
- `--rate-limit` how many commands a client can send in a second.
- `--rate-step` how many commands per second the rate limit should be increased by.
- `--rate-step-interval` how often the rate limit should be stepped up, as a duration string.

Using the rate limit and the rate step flags will put a cap on how many commands the client can send each second.
Then, this cap can be increased by the step value at regular intervals. Ultimately, the client will send commands at a rate
that is faster than the replicas can process, and hit the max-concurrent limit.

### Replica flags

- `--batch-size` the number of client commands that should be batched together in a block.
- `--view-timeout` the initial setting for the view duration.
  In other words, the view-synchronizers will timeout the first view after this duration has passed.
  Subsequent views may have longer or shorter timeouts.
- `--max-timeout` an upper limit on the view timeout. The view-synchronizers will not wait any longer than this duration.
- `--timeout-multiplier` the number that the old view duration value should be multiplied by when a timeout occurs.
- `--duration-samples` the number of previous views that should be sampled to calculate the view timeout.

The different timeout flags together control the behavior of the view synchronizer module.
The initial timeout is set by the `view-timeout` flag, which only influences the first few views.
The `timeout-multiplier` sets the multiplier that is used when a view timeout occurs.
If the view duration is 1 second and the timeout-multiplier is 2, then if a timeout occurs,
the next view will have a timeout of 2 seconds instead.

### Module flags

- `--consensus` the name of the consensus implementation to use. Currently, the valid values are `chainedhotstuff`,
  `fasthotstuff`, and `simplehotstuff`.
- `--crypto` the name of the crypto implementation to use. The valid options are `ecdsa` and `bls12`.
- `--leader-rotation` the name of the leader-rotation implementation to use. Currently, the valid values are
  `round-robin` and `fixed`.

### Metrics flags

- `--metrics` the list of metrics to enable. This should be a comma separated list of names that correspond to metrics
  implementations that are registered with the `metrics` package.
- `--output` the path to a directory where measurements and other output should be saved.
- `--measurement-interval` configures the interval of the ticker module, which most metrics use to determine how often
  to log measurements.

### Performance monitoring flags

The following flags also create files in the directory specified by the `output` flag.

- `--cpu-profile` enables a CPU profile.
- `--mem-profile` enables a memory profile.
- `--fgprof-profile` enables profile using the [`fgprof` package](https://github.com/felixge/fgprof).
- `--trace` enables a trace.

That covers the relevant flags for running local tests. The rest of the flags are relevant for when running tests on
remote hosts, which is what we will cover next.

## Running experiments on remote hosts

The cli supports connecting to remote hosts via SSH to run experiments.
There are, however, some limitations. First, you must use public key authentication.
Second, all hosts that you connect to must have an entry in a `known_hosts` file.
Third, the hosts must support SFTP and should provide a unix-like environment.

You need to provide a ssh config file that the cli will use to get connection info.
The cli uses the [`ssh_config` package](https://github.com/kevinburke/ssh_config) to read these files,
which does not support all possible configuration options.
However, the main ones you might want to specify are:

- `User`
- `Port`
- `IdentityFile`
- `UserKnownHostsFile`

For example:

```text
Host 127.0.0.1 localhost
    User root
    Port 2020
    IdentityFile ./scripts/id
    UserKnownHostsFile ./scripts/known_hosts
```

All flags discussed in the previous section also apply to remote experiments, but for remote experiments,
these additional flags are used:

- `--exe` specifies the path to the executable that should be deployed to the remote hosts. This should be used if the
  remote hosts use a different operating system or architecture than the local host, otherwise the cli will deploy itself
  using the currently executing binary.
- `--ssh-config` specifies the path to the ssh config file to use. If this flag is not specified,
  the cli will read `~/.ssh/config` instead.
- `--hosts` a comma separated list of hosts to connect to. It is preferable to use host names that have entries in the ssh config file.
- `--worker` runs a worker locally, in addition to the remote hosts specified. Use this if you want the local machine
  to participate in the experiment.

Additionally, it is possible to specify an *internal address* for each host.
The internal address is used by replicas instead of the address used by the controller.
This is useful if the controller is connecting to the remote hosts using a global address,
whereas the hosts can communicate using local addresses.
The internal address is configured through the configuration file (loaded by the `--config` flag):

```toml
[[hosts-config]]
name = "hotstuff_worker_1"
internal-address = "192.168.10.2"

[[hosts-config]]
name = "hotstuff_worker_1"
internal-address = "192.168.10.3"
```

### Manual assignment of clients and replicas

By default, the controller (the local machine) will divide clients and replicas as evenly as possible among all workers
(the remote hosts). You can override this behavior by specifying how many clients and replicas should be assigned to
each host individually. This can only be done through the configuration file, not through command-line flags.
The following shows a configuration file that customizes the client and replica assignment for one of the hosts:

```toml
clients = 2
replicas = 8

hosts = [ 
    "hotstuff_worker_1",
    "hotstuff_worker_2",
    "hotstuff_worker_3",
    "hotstuff_worker_4",
]

# specific assignments for some hosts
[[hosts-config]]
name = "hotstuff_worker_1"
clients = 2
replicas = 0
```

In particular, in this example the host named `hotstuff_worker_1` is configured to run both clients and no replicas.
The remaining replicas are divided among the remaining hosts. If all hosts are manually configured, the total number of
clients and replicas configured must equal the requested number of clients and replicas.

## Plotting measurements

We have implemented a very basic plotting program that can plot some of the metrics.
This program is also compiled using `make`, and you can see all of its options by running `./plot --help`.
It supports multiple output formats, such as pdf, png, and csv.
