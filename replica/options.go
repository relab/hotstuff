package replica

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type replicaOptions struct {
	credentials          credentials.TransportCredentials
	clientGorumsSrvOpts  []gorums.ServerOption
	replicaGorumsSrvOpts []gorums.ServerOption
	cmdCacheOpts         []clientpb.CommandCacheOption
	serverOpts           []server.ServerOption
}

func newDefaultOpts() *replicaOptions {
	return &replicaOptions{
		credentials: insecure.NewCredentials(),
	}
}

type Option func(*replicaOptions)

func WithCmdCacheOptions(opts ...clientpb.CommandCacheOption) Option {
	return func(ro *replicaOptions) {
		ro.cmdCacheOpts = append(ro.cmdCacheOpts, opts...)
	}
}

func WithServerOptions(opts ...server.ServerOption) Option {
	return func(ro *replicaOptions) {
		ro.serverOpts = append(ro.serverOpts, opts...)
	}
}

func WithTLS(certificate tls.Certificate, rootCAs *x509.CertPool, creds credentials.TransportCredentials) Option {
	return func(ro *replicaOptions) {
		ro.clientGorumsSrvOpts = append(ro.clientGorumsSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewServerTLSFromCert(&certificate)),
		))
		ro.replicaGorumsSrvOpts = append(ro.replicaGorumsSrvOpts, gorums.WithGRPCServerOptions(grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{certificate},
			ClientCAs:    rootCAs,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}))))
		ro.credentials = creds
	}
}
