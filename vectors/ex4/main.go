package main

import (
	"fmt"
)

//gown: observer fmt.Printf

type wheel struct{}

type bicycle struct {
	front *wheel //gown:iso
}

// dispose of goner *wheel, replace with spare
// gown: param goner iso
func puncture(goner *wheel) {}

// gown: result answer iso
func f() (answer *bicycle) {
	return &bicycle{} //gown:new
}

func main() {
	j := &bicycle{ //gown:iso
		front: &wheel{},
	}

	var a *wheel            //gown: iso
	a, j.front = j.front, a // this is an auto-swap test case

	fmt.Printf("a='%#v'\n", a)
	fmt.Printf("j.front='%#v'\n", j.front)
}
