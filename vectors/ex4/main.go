package main

import (
	"fmt"
)

//gown: observer fmt.Printf

type wheel struct{}

type bicycle struct {
	front *wheel //gown:iso
}

// gown: param goner iso
func puncture(goner *wheel) {}

// gown: result answer iso
func f() (answer *bicycle) {
	return &bicycle{} //gown:new
}

// gown: result answer iso
func ff() (anum int, answer *bicycle) {
	return 7, &bicycle{} //gown:new
}

// gown: result 0 iso
func g() *bicycle {
	return &bicycle{} //gown:new
}

// gown: result 1 iso
func h() (int, *bicycle) {
	return 12, &bicycle{} //gown:new
}

// gown: result 2 iso
func gg() (int, int, *bicycle, string) {
	return 1,
		2,
		&bicycle{}, //gown:new
		"this string is last"
}

// gown: result 3 iso
func ggg() (int, int, string, *bicycle) {
	return 1, 2, "new bikes are fun", &bicycle{} //gown:new
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
