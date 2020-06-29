module github.com/relab/hotstuff

go 1.13

require (
	github.com/golang/protobuf v1.4.2
	github.com/google/gofuzz v1.1.0
	github.com/relab/gorums v0.2.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2
	go-hep.org/x/hep v0.27.0
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	golang.org/x/text v0.3.3 // indirect
	gonum.org/v1/plot v0.7.1-0.20200414075901-f4e1939a9e7a
	google.golang.org/genproto v0.0.0-20200701001935-0939c5918c31 // indirect
	google.golang.org/grpc v1.30.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v0.0.0-20200630190442-3de8449f8555 // indirect
	google.golang.org/protobuf v1.25.0
)

replace github.com/relab/gorums => github.com/Raytar/gorums v0.0.0-20200629103911-ebd0464a1930
