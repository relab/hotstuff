package orchestration

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/google/wire"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/network/sender"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/committer"
	"google.golang.org/grpc/credentials"
)

type CacheSizeType int

func newCertAuth(
	cryptoImpl modules.CryptoBase,
	blockChain *blockchain.BlockChain,
	logger logging.Logger,
	cacheSize CacheSizeType) *certauth.CertAuthority {
	if cacheSize > 0 {
		return certauth.NewCached(
			cryptoImpl,
			blockChain,
			logger,
			int(cacheSize),
		)
	}

	return certauth.New(
		cryptoImpl,
		blockChain,
		logger,
	)
}

type BatchSizeType int

func newCmdCache(logger logging.Logger, batchSize BatchSizeType) *clientsrv.CmdCache {
	return clientsrv.NewCmdCache(logger, int(batchSize))
}

type BufferSizeType uint

func newEventLoop(logger logging.Logger, bufferSize BufferSizeType) *eventloop.EventLoop {
	return eventloop.New(logger, uint(bufferSize))
}

func Initialize(
	cryptoImpl modules.CryptoBase,
	consensusRules modules.ConsensusRules,
	leaderRotation modules.LeaderRotation,
	viewDuration modules.ViewDuration,
	id hotstuff.ID,
	privKey hotstuff.PrivateKey,
	bufferSize BufferSizeType,
	name string,
	creds credentials.TransportCredentials,
	cacheSize CacheSizeType,
	batchSize BatchSizeType,
	clientSrvOpts []gorums.ServerOption,
	opts *orchestrationpb.ReplicaOpts,
	certificate tls.Certificate,
	rootCAs *x509.CertPool,
) (*synchronizer.Synchronizer, error) {
	wire.Build(
		netconfig.NewConfig,
		core.NewOptions,
		logging.New,
		newEventLoop,
		sender.New,
		blockchain.New,
		newCertAuth,
		newCmdCache,
		clientsrv.NewClientServer,
		committer.New,
		consensus.New,
		synchronizer.New,
	)
	return nil, nil
}
