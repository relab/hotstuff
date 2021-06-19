module github.com/relab/hotstuff

go 1.16

require (
	github.com/Raytar/iago v0.0.0
	github.com/felixge/fgprof v0.9.1
	github.com/golang/mock v1.5.0
	github.com/golang/protobuf v1.5.2
	github.com/kilic/bls12-381 v0.1.1-0.20210208205449-6045b0235e36
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/go-homedir v1.1.0
	github.com/relab/gorums v0.5.1-0.20210618201939-a065aa7e4060
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	go.uber.org/multierr v1.6.0
	go.uber.org/zap v1.16.0
	google.golang.org/grpc v1.37.0
	google.golang.org/protobuf v1.26.0
)

replace github.com/Raytar/iago => ../iago

replace github.com/Raytar/wrfs => ../wrfs
