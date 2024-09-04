package pipelining

type PipeId uint32

const NullPipeId = PipeId(0)

// If the pipe ID is not NullPipeId, then return true
func ValidPipe(pipeId PipeId) bool {
	return pipeId != NullPipeId
}

// If the list contains duplicate IDs or a NullPipeId, then return false
func ValidPipes(pipeIds []PipeId) bool {
	for i, pId := range pipeIds {
		if !ValidPipe(pId) {
			return false
		}

		for j, otherPid := range pipeIds {
			if i != j && otherPid == pId {
				return false
			}
		}
	}
	return true
}
