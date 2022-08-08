package tools

func PtrTo[T any](o T) *T {
	return &o
}
