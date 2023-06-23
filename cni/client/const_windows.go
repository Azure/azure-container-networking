package client

// CNIExecDir is the working directory that the invoker must execute the CNI from
// in order for it to correctly map its state and lock files.
// Only needs to be set on Windows.
const CNIExecDir = "C:\\k"
