package util

func CloneBytes(in []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
