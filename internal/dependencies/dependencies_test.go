package dependencies_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/dependencies"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	"github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
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
			leaderRotationName: leaderrotation.RoundRobinModuleName,
			byzantineStrategy:  byzantine.ForkModuleName,
		},
		{
			cryptoName:         eddsa.ModuleName,
			consensusName:      simplehotstuff.ModuleName,
			leaderRotationName: leaderrotation.FixedModuleName,
			byzantineStrategy:  byzantine.SilenceModuleName,
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      fasthotstuff.ModuleName,
			leaderRotationName: leaderrotation.CarouselModuleName,
			byzantineStrategy:  "",
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      fasthotstuff.ModuleName,
			leaderRotationName: leaderrotation.ReputationModuleName,
			byzantineStrategy:  "",
		},
		{
			cryptoName:         bls12.ModuleName,
			consensusName:      fasthotstuff.ModuleName,
			leaderRotationName: leaderrotation.TreeLeaderModuleName,
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
		depsCore := dependencies.NewCore(hotstuff.ID(1), "test", pk, 0)
		depsNet := dependencies.NewNetwork(depsCore, insecure.NewCredentials())
		depsSecure, err := dependencies.NewSecurity(depsCore, depsNet, td.cryptoName, 100)
		if err != nil {
			t.Fatalf("%v", err)
		}
		depsSrv := dependencies.NewService(depsCore, depsSecure, 1)
		_, err = dependencies.NewProtocol(
			depsCore, depsNet, depsSecure, depsSrv,
			td.consensusName, td.leaderRotationName, td.byzantineStrategy,
			viewduration.NewParams(0, 0, 0, 0))
		if err != nil {
			t.Fatalf("%v", err)
		}
	}
}
