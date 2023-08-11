package main

import (
	"fmt"
)

func main() {
	err := fmt.Errorf("no wrapping verb: %d", 42)
	fmt.Println("error:", err.Error())
}

// Output:
