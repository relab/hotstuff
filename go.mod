module github.com/relab/hotstuff

go 1.13

require (
	github.com/google/gofuzz v1.1.0
	github.com/relab/gorums v0.1.0-gogo
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.6.2
	golang.org/x/net v0.0.0-20200202094626-16171245cfb2
	golang.org/x/sys v0.0.0-20200202164722-d101bd2416d5 // indirect
	golang.org/x/text v0.3.2 // indirect
	google.golang.org/genproto v0.0.0-20200204135345-fa8e72b47b90 // indirect
	google.golang.org/grpc v1.28.0-pre.0.20200205181625-597699c0ef29
	google.golang.org/protobuf v1.20.1
)

replace github.com/relab/gorums => github.com/Raytar/gorums v0.0.0-20200205230032-26597049232a
