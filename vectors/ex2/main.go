package main

import (
	"fmt"
)

//gown: observer fmt.Printf

type wheel struct{}

type bicycle struct {
	front *wheel //gown:iso
}

// dispose of goner *wheel
// gown: param goner iso
func puncture(goner *wheel) {}

func main() {
	j := &bicycle{
		front: &wheel{},
	}

	a := j

	puncture(a.front) // this should not be allowed since a retains a.front afterwards

	fmt.Printf("a='%#v'", a)
}
