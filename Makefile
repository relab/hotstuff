.PHONY: proto
proto:
	protoc -I=${GOPATH}/src:. --gorums_out=plugins=grpc+gorums:proto hotstuff.proto