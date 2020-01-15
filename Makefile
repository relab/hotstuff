.PHONY: proto
proto:
	protoc -I=${GOPATH}/src:. --gorums_out=plugins=grpc+gorums:pkg/proto hotstuff.proto
