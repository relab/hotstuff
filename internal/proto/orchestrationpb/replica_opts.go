package orchestrationpb

import (
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/keygen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// New creates a new ReplicaOpts with the given replicaID and locations.
// The locations are shared between all replicas, and are indexed by the replicaID.
func (x *ReplicaOpts) New(replicaID hotstuff.ID, locations []string) *ReplicaOpts {
	replicaOpts := proto.Clone(x).(*ReplicaOpts)
	replicaOpts.ID = uint32(replicaID)
	replicaOpts.Locations = locations
	return replicaOpts
}

// TreePositionIDs returns the tree positions as a slice of hotstuff.ID.
func (x *ReplicaOpts) TreePositionIDs() []hotstuff.ID {
	ids := make([]hotstuff.ID, len(x.GetTreePositions()))
	for i, id := range x.GetTreePositions() {
		ids[i] = hotstuff.ID(id)
	}
	return ids
}

// TreeDeltaDuration returns the tree delta as a time.Duration.
func (x *ReplicaOpts) TreeDeltaDuration() time.Duration {
	return x.GetTreeDelta().AsDuration()
}

func (x *ReplicaOpts) SetTreeOptions(branchFactor uint32, positions []uint32, treeDelta time.Duration) {
	x.TreePositions = positions
	x.BranchFactor = branchFactor
	x.TreeDelta = durationpb.New(treeDelta)
}

func (x *ReplicaOpts) SetByzantineStrategy(strategy string) {
	x.ByzantineStrategy = strategy
}

func (x *ReplicaOpts) HotstuffID() hotstuff.ID {
	return hotstuff.ID(x.GetID())
}

func (x *ReplicaOpts) SetReplicaCertificates(host string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	x.CertificateAuthority = keygen.CertToPEM(ca)
	validFor := []string{"localhost", "127.0.0.1", "127.0.1.1", host}
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			if ipStr := ip.String(); ipStr != host {
				validFor = append(validFor, ipStr)
			}
		}
	}

	keyChain, err := keygen.GenerateKeyChain(hotstuff.ID(x.ID), validFor, x.Crypto, ca, caKey)
	if err != nil {
		return fmt.Errorf("failed to generate keychain: %w", err)
	}
	x.PrivateKey = keyChain.PrivateKey
	x.PublicKey = keyChain.PublicKey
	x.Certificate = keyChain.Certificate
	x.CertificateKey = keyChain.CertificateKey
	return nil
}

func (x *ReplicaOpts) StringID() string {
	return strconv.Itoa(int(x.GetID()))
}

func (x *ReplicaOpts) StringLocations() string {
	s := strings.Builder{}
	s.WriteString("ID: ")
	s.WriteString(x.StringID())
	s.WriteString(", ")
	s.WriteString("Locations: ")
	s.WriteString(strings.Join(x.GetLocations(), ", "))
	return s.String()
}
