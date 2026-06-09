package main

type pointy *int

type trickySlice []*int

func main() {

	a := make(map[pointy]int)  //gown: iso
	b := make(trickySlice, 10) //gown: iso
	_, _ = a, b
}
