package main

import (
	"fmt"
)

//gown:observer fmt.Printf

type Job struct {
	Input *int //gown:iso
}

func main() {
	nine := new(int) //gown:iso
	*nine = 9
	j := &Job{ //gown:new
		Input: nine,
		//func() *int {
		//			gownNew := int(9)
		//			return &gownNew
		//		}(),
	}

	fmt.Printf("j='%#v'", j)

	// hopefully not allowed some how:
	//k := &nine //gown:iso
	//_ = k
}
