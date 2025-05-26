package dependencies_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/dependencies"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/leaderrotation/carousel"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/leaderrotation/reputation"
	"github.com/relab/hotstuff/protocol/leaderrotation/roundrobin"
	"github.com/relab/hotstuff/protocol/leaderrotation/treeleader"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	"github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
	"google.golang.org/grpc/credentials/insecure"
)

func TestModules(t *testing.T) {
	testData := []struct {
		cryptoName         string
		consensusName      string
		leaderRotationName string
		byzantineStrategy  string
	}{
		{
			cryptoName:         ecdsa.ModuleName,
			consensusName:      chainedhotstuff.ModuleName,
			leaderRotationName: roundrobin.ModuleName,
			byzantineStrategy:  byzantine.ForkModuleName,
		},
		{
			cryptoName:         eddsa.ModuleName,
			consensusName:      simplehotstuff.ModuleName,
			leaderRotationName: fixedleader.ModuleName,
			byzantineStrategy:  byzantine.SilenceModuleName,
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      fasthotstuff.ModuleName,
			leaderRotationName: carousel.ModuleName,
			byzantineStrategy:  "",
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      fasthotstuff.ModuleName,
			leaderRotationName: reputation.ModuleName,
			byzantineStrategy:  "",
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      fasthotstuff.ModuleName,
			leaderRotationName: treeleader.ModuleName,
			byzantineStrategy:  "",
		},
	}

	for _, td := range testData {
		var pk hotstuff.PrivateKey
		switch td.cryptoName {
		case bls12.ModuleName:
			pk = testutil.GenerateBLS12Key(t)
		case ecdsa.ModuleName:
			pk = testutil.GenerateECDSAKey(t)
		case eddsa.ModuleName:
			pk = testutil.GenerateEDDSAKey(t)
		}
		runtimeOpts := []core.RuntimeOption{}
		if td.consensusName == fasthotstuff.ModuleName {
			runtimeOpts = append(runtimeOpts, core.WithAggregateQC())
		}
		depsCore := dependencies.NewCore(hotstuff.ID(1), "test", pk, runtimeOpts...)
		sender := network.NewGorumsSender(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			insecure.NewCredentials(),
		)
		depsSecure, err := dependencies.NewSecurity(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			sender,
			td.cryptoName,
			certauth.WithCache(100),
		)
		if err != nil {
			t.Fatalf("%v", err)
		}
		_ = depsSecure
		// TODO(AlanRostem): fix
		/*depsSrv := dependencies.NewService(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsSecure.BlockChain(),
			nil,
		)
		_, err = dependencies.NewProtocol(
			depsCore,
			depsNet,
			depsSecure,
			depsSrv,
			td.consensusName,
			td.leaderRotationName,
			td.byzantineStrategy,
			viewduration.NewParams(0, 0, 0, 0))
		if err != nil {
			t.Fatalf("%v", err)
		}*/
	}
}
