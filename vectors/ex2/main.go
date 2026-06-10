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

// gown: func puncture(goner \iso *wheel)
func puncture(goner *wheel) {}

func main() {
	j := &bicycle{
		front: &wheel{},
	}

	a := j

	puncture(a.front)

	b := a.front // this should not be allowed since a.front was consumed in puncture()

	fmt.Printf("b='%#v'", b)
}
