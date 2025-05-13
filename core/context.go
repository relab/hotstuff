package core

import (
	"context"
	"fmt"
	"strconv"

	"github.com/relab/hotstuff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// PeerIDFromContext extracts the ID of the peer from the context.
func (g *RuntimeConfig) PeerIDFromContext(ctx context.Context) (hotstuff.ID, error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return 0, fmt.Errorf("peerInfo not available")
	}

	if peerInfo.AuthInfo != nil && peerInfo.AuthInfo.AuthType() == "tls" {
		tlsInfo, ok := peerInfo.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return 0, fmt.Errorf("authInfo of wrong type: %T", peerInfo.AuthInfo)
		}
		if len(tlsInfo.State.PeerCertificates) > 0 {
			cert := tlsInfo.State.PeerCertificates[0]
			for replicaID := range g.replicas {
				if subject, err := strconv.Atoi(cert.Subject.CommonName); err == nil && hotstuff.ID(subject) == replicaID {
					return replicaID, nil
				}
			}
		}
		return 0, fmt.Errorf("could not find matching certificate")
	}

	// If we're not using TLS, we'll fallback to checking the metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, fmt.Errorf("metadata not available")
	}

	v := md.Get("id")
	if len(v) < 1 {
		return 0, fmt.Errorf("id field not present")
	}

	id, err := strconv.Atoi(v[0])
	if err != nil {
		return 0, fmt.Errorf("cannot parse ID field: %w", err)
	}

	return hotstuff.ID(id), nil
}
