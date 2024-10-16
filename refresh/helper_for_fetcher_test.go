package refresh

func (f *Fetcher[T]) SetTicker(t TickProvider) {
	f.ticker = t
}
