package client

// CNIExecDir is the working directory that the invoker must execute the CNI from
// in order for it to correctly map its state and lock files.
// Does not need to be set on Linux, as absolute paths are correctly used during
// the actual CNI execution.
const CNIExecDir = ""
