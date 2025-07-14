package infiniband

type Status int

const (
	ProgrammingPending  Status = 0
	ProgrammingFailed   Status = 1
	ProgrammingComplete Status = 2
	ReleasePending      Status = 3
	Available           Status = 4
)
