package thing_test

import (
	"fmt"

	"example.com/thing"
)

func ExampleAdd() {
	fmt.Println(thing.Add(2, 3))
	// Output: 5
}

// A counter counts from one.
func ExampleCounter_inc() {
	c := thing.NewCounter()
	fmt.Println(c.Inc())
	// Output: 1
}
