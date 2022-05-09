package internal

import (
	"fmt"
	"time"
)

func ExampleExponential() {
	// this example details the common case where the powers of 2 are desired
	cooldown := Exponential(1*time.Millisecond, 2)()

	for i := 0; i < 5; i++ {
		got, err := cooldown()
		if err != nil {
			fmt.Println("received error during cooldown: err:", err)
			return
		}

		fmt.Println(got)
	}

	// Output:
	// 1ms
	// 2ms
	// 4ms
	// 8ms
	// 16ms
}

func ExampleFixed() {
	cooldown := Fixed(10 * time.Millisecond)()

	for i := 0; i < 5; i++ {
		got, err := cooldown()
		if err != nil {
			fmt.Println("unexpected error cooling down: err", err)
			return
		}
		fmt.Println(got)

		// Output:
		// 10ms
		// 10ms
		// 10ms
		// 10ms
		// 10ms
	}
}

func ExampleMax() {
	cooldown := Max(4, Fixed(10*time.Millisecond))()

	for i := 0; i < 5; i++ {
		got, err := cooldown()
		if err != nil {
			fmt.Println("error cooling down:", err)
			break
		}
		fmt.Println(got)

		// Output:
		// 10ms
		// 10ms
		// 10ms
		// 10ms
		// error cooling down: maximum attempts reached
	}
}
