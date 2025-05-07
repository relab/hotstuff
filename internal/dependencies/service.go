package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/committer"
)

type Service struct {
	CmdCache  *clientsrv.CmdCache
	ClientSrv *clientsrv.ClientServer
	Committer *committer.Committer
}

func NewService(
	coreComps *Core,
	secureComps *Security,
	batchSize int,
	clientSrvOpts []gorums.ServerOption,
) *Service {
	cmdCache := clientsrv.NewCmdCache(
		coreComps.Logger,
		batchSize,
	)
	clientSrv := clientsrv.NewClientServer(
		coreComps.EventLoop,
		coreComps.Logger,
		cmdCache,
		clientSrvOpts,
	)
	committer := committer.New(
		secureComps.BlockChain,
		clientSrv,
		coreComps.Logger,
	)
	return &Service{
		CmdCache:  cmdCache,
		ClientSrv: clientSrv,
		Committer: committer,
	}
}
