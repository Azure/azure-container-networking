package infiniband

type Status int

// IBStatus definitions
const (
	PendingProgramming Status = 0
	Error              Status = 1
	Programmed         Status = 2
	PendingDeletion    Status = 3
	Available          Status = 4
)
