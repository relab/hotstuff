package pipelining

type PipeId uint32

const NullPipeId = PipeId(0)

// If the pipe ID is not NullPipeId, returns true
func ValidId(pipeId PipeId) bool {
	return pipeId != NullPipeId
}

// If the list contains duplicate IDs or a NullPipeId, return false
func ValidIds(pipeIds []PipeId) bool {
	for i, pId := range pipeIds {
		if !ValidId(pId) {
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
