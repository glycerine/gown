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
	j := &bicycle{ //gown:iso
		front: &wheel{},
	}

	var a *wheel //gown: iso
	a, j.front = j.front, a

	fmt.Printf("a='%#v'\n", a)
	fmt.Printf("j.front='%#v'\n", j.front)
}
