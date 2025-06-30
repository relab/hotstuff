package hotstuff

import "math"

// NumFaulty returns the maximum number of faulty replicas in a system with n replicas.
func NumFaulty(n int) int {
	return (n - 1) / 3
}

// QuorumSize returns the minimum number of replicas that must agree on a value for it to be considered a quorum.
func QuorumSize(n int) int {
	f := NumFaulty(n)
	return int(math.Ceil(float64(n+f+1) / 2.0))
}
