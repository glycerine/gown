package main

import (
	"fmt"
)

//gown: observer fmt.Printf

type wheel struct{}

type bicycle struct {
	front *wheel //gown:iso
}

// gown: func puncture(goner \iso *wheel)
func puncture(goner *wheel) {}

type shop struct{}

// gown: func (s *shop) f() (answer \iso *bicycle)
func (s *shop) f() (answer *bicycle) {
	return &bicycle{} //gown:new
}

// gown: ff() (anum int, answer \iso *bicycle)
func ff() (anum int, answer *bicycle) {
	return 7, &bicycle{} //gown:new
}

// gown: func g() \iso *bicycle {
func g() *bicycle {
	return &bicycle{} //gown:new
}

// gown: func h() (int, \iso *bicycle) {
func h() (int, *bicycle) {
	return 12, &bicycle{} //gown:new
}

// gown: func gg() (int, int, \iso *bicycle, string) {
func gg() (int, int, *bicycle, string) {
	return 1,
		2,
		&bicycle{}, //gown:new
		"this string is last"
}

//gown:func ggg() (int, int, string, \iso *bicycle) {
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
