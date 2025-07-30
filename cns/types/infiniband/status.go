package infiniband

type Status int

const (
	Available           Status = 0
	ProgrammingPending  Status = 1
	ProgrammingFailed   Status = 2
	ProgrammingComplete Status = 3
	ReleasePending      Status = 4
)
