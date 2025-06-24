package wiring_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
	"github.com/relab/hotstuff/wiring"
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
			consensusName:      rules.ModuleNameChainedHotstuff,
			leaderRotationName: leaderrotation.ModuleNameRoundRobin,
			byzantineStrategy:  byzantine.ModuleNameFork,
		},
		{
			cryptoName:         eddsa.ModuleName,
			consensusName:      rules.ModuleNameSimpleHotStuff,
			leaderRotationName: leaderrotation.ModuleNameFixed,
			byzantineStrategy:  byzantine.ModuleNameSilence,
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      rules.ModuleNameFastHotstuff,
			leaderRotationName: leaderrotation.ModuleNameCarousel,
			byzantineStrategy:  "",
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      rules.ModuleNameFastHotstuff,
			leaderRotationName: leaderrotation.ModuleNameReputation,
			byzantineStrategy:  "",
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      rules.ModuleNameFastHotstuff,
			leaderRotationName: leaderrotation.ModuleNameTree,
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
		if td.consensusName == rules.ModuleNameFastHotstuff {
			runtimeOpts = append(runtimeOpts, core.WithAggregateQC())
		}
		depsCore := wiring.NewCore(hotstuff.ID(1), "test", pk, runtimeOpts...)
		sender := network.NewGorumsSender(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			insecure.NewCredentials(),
		)
		depsSecure, err := wiring.NewSecurity(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			sender,
			td.cryptoName,
			cert.WithCache(100),
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
