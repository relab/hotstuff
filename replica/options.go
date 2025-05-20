package replica

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/service/cmdcache"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type moduleNames struct {
	crypto,
	consensus,
	leaderRotation,
	byzantineStrategy string
}

type replicaOptions struct {
	credentials          credentials.TransportCredentials
	clientGorumsSrvOpts  []gorums.ServerOption
	replicaGorumsSrvOpts []gorums.ServerOption
	cmdCacheOpts         []cmdcache.Option
	serverOpts           []server.ServerOption
	moduleNames          moduleNames
	withKauri            bool
}

func newDefaultOpts() *replicaOptions {
	return &replicaOptions{
		credentials: insecure.NewCredentials(),
		moduleNames: moduleNames{
			crypto:            ecdsa.ModuleName,
			consensus:         chainedhotstuff.ModuleName,
			leaderRotation:    leaderrotation.RoundRobinModuleName,
			byzantineStrategy: "",
		},
		withKauri: false,
	}
}

type Option func(*replicaOptions)

// OverrideCrypto changes the crypto implementation to the one specified by name.
// Default: "ecdsa"
func OverrideCrypto(moduleName string) Option {
	return func(ro *replicaOptions) {
		ro.moduleNames.crypto = moduleName
	}
}

// OverrideConsensusRules changes the consensus rules to the one specified by name.
// Default: "chainedhotstuff"
func OverrideConsensusRules(moduleName string) Option {
	return func(ro *replicaOptions) {
		ro.moduleNames.consensus = moduleName
	}
}

// OverrideLeaderRotation changes the leader rotation scheme to the one specified by name.
// Default: "round-robin"
func OverrideLeaderRotation(moduleName string) Option {
	return func(ro *replicaOptions) {
		ro.moduleNames.leaderRotation = moduleName
	}
}

// WithByzantineStrategy wraps the existing consensus rules to a byzantine ruleset specified by name.
func WithByzantineStrategy(strategyName string) Option {
	return func(ro *replicaOptions) {
		ro.moduleNames.byzantineStrategy = strategyName
	}
}

func WithCmdCacheOptions(opts ...cmdcache.Option) Option {
	return func(ro *replicaOptions) {
		ro.cmdCacheOpts = append(ro.cmdCacheOpts, opts...)
	}
}

func WithServerOptions(opts ...server.ServerOption) Option {
	return func(ro *replicaOptions) {
		ro.serverOpts = append(ro.serverOpts, opts...)
	}
}

func WithTLS(certificate tls.Certificate, rootCAs *x509.CertPool) Option {
	return func(ro *replicaOptions) {
		creds := credentials.NewTLS(&tls.Config{
			RootCAs:      rootCAs,
			Certificates: []tls.Certificate{certificate},
		})
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

func WithKauri() Option {
	return func(ro *replicaOptions) {
		ro.withKauri = true
	}
}
