module github.com/relab/hotstuff

go 1.16

require (
	github.com/felixge/fgprof v0.9.1
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/golang/mock v1.5.0
	github.com/golang/protobuf v1.5.2
	github.com/kilic/bls12-381 v0.1.1-0.20210208205449-6045b0235e36
	github.com/kr/pretty v0.2.1 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pelletier/go-toml v1.8.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/relab/gorums v0.5.0
	github.com/spf13/afero v1.2.2 // indirect
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.6.1 // indirect
	go.uber.org/multierr v1.6.0
	go.uber.org/zap v1.16.0
	google.golang.org/grpc v1.37.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	honnef.co/go/tools v0.0.1-2020.1.3 // indirect
)

replace github.com/Raytar/iago => ../iago

replace github.com/Raytar/wrfs => ../wrfs

replace github.com/relab/gorums => ../gorums
