package pipeline

type Pipe uint32

const NullPipe = Pipe(0)

// If the pipe ID is not NullPipeId, then return true
func ValidPipe(pipeId Pipe) bool {
	return pipeId != NullPipe
}

// If the list contains duplicate IDs or a NullPipeId, then return false
func ValidPipes(pipeIds []Pipe) bool {
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
