package util

// CopyPointersValue takes a pointer and shallow copies the underlying value, returning a pointer to the copy. If in is nil, CopyPointersValue returns nil.
func CopyPointersValue[T any](in *T) *T {
	if in == nil {
		return nil
	}
	return new(*in)
}
