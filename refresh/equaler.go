package refresh

type equaler[T any] interface {
	Equal(T) bool
}
