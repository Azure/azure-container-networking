package util

import (
	"fmt"
	"hash/fnv"
)

// Hash hashes a string to another string with length <= 32.
func Hash(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprint(h.Sum32())
}
