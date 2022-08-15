package hotstuffpb

func (sig *ECDSASignature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.GetR()...)
	b = append(b, sig.GetS()...)
	return b
}
