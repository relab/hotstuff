module github.com/relab/hotstuff

go 1.13

require (
	github.com/google/gofuzz v1.1.0
	github.com/relab/gorums v0.1.0-gogo
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a
	golang.org/x/sys v0.0.0-20200317113312-5766fd39f98d // indirect
	golang.org/x/text v0.3.2 // indirect
	google.golang.org/genproto v0.0.0-20200317114155-1f3552e48f24 // indirect
	google.golang.org/grpc v1.28.0
	google.golang.org/protobuf v0.0.0-20200109180630-ec00e32a8dfd
)

replace github.com/relab/gorums => github.com/Raytar/gorums v0.0.0-20200317123200-f7376b477596
