package orchestration

import (
	"testing"

	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
)

type testResponsesData struct {
	responses     []*orchestrationpb.StopReplicaResponse
	expectedError bool
}

var testData = []testResponsesData{
	{
		responses: []*orchestrationpb.StopReplicaResponse{
			{
				Hashes: map[uint32][]byte{
					1: []byte("execution_hash"),
					2: []byte("execution_hash"),
					3: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					1: 100,
					2: 100,
					3: 100,
				},
			},
			{
				Hashes: map[uint32][]byte{
					4: []byte("execution_hash"),
					5: []byte("execution_hash1"),
					6: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					4: 100,
					5: 200,
					6: 100,
				},
			},
			{
				Hashes: map[uint32][]byte{
					7: []byte("execution_hash"),
					8: []byte("execution_hash1"),
					9: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					7: 100,
					8: 200,
					9: 100,
				},
			},
		},
		expectedError: false,
	},
	{
		responses: []*orchestrationpb.StopReplicaResponse{
			{
				Hashes: map[uint32][]byte{
					1: []byte("execution_hash"),
					2: []byte("execution_hash"),
					3: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					1: 100,
					2: 200,
					3: 100,
				},
			},
			{
				Hashes: map[uint32][]byte{
					4: []byte("execution_hash"),
					5: []byte("execution_hash1"),
					6: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					4: 100,
					5: 200,
					6: 100,
				},
			},
			{
				Hashes: map[uint32][]byte{
					7: []byte("execution_hash"),
					8: []byte("execution_hash1"),
					9: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					7: 100,
					8: 200,
					9: 100,
				},
			},
		},
		expectedError: true,
	},
	{
		responses: []*orchestrationpb.StopReplicaResponse{
			{
				Hashes: map[uint32][]byte{
					1: []byte("execution_hash"),
					2: []byte("execution_hash"),
					3: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					1: 100,
					2: 100,
					3: 100,
				},
			},
			{
				Hashes: map[uint32][]byte{
					4: []byte("execution_hash"),
					5: []byte("execution_hash2"),
					6: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					4: 100,
					5: 200,
					6: 100,
				},
			},
			{
				Hashes: map[uint32][]byte{
					7: []byte("execution_hash"),
					8: []byte("execution_hash1"),
					9: []byte("execution_hash"),
				},
				Counts: map[uint32]uint32{
					7: 100,
					8: 200,
					9: 100,
				},
			},
		},
		expectedError: true,
	},
}

func TestCorrectStopReplicaResponses(t *testing.T) {
	for _, testCase := range testData {
		err := verifyStopResponses(testCase.responses)
		switch {
		case err == nil && testCase.expectedError:
			t.Errorf("Failed to return error with invalid responses")
		case err != nil && !testCase.expectedError:
			t.Errorf("Unexpected error with valid responses: %v", err)
		}
	}
}
