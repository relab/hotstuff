package hotstuff

// NumFaulty calculates 'f', which is the number of replicas that can be faulty for a configuration of size 'n'.
func NumFaulty(n int) int {
	return (n - 1) / 3
}

// QuorumSize calculates '2f + 1', which is the quorum size for a configuration of size 'n'.
func QuorumSize(n int) int {
	return n - NumFaulty(n)
}
