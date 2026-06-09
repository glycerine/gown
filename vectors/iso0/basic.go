package main

type payload struct {
	dirt string
}

func main() {

	ch := make(chan *payload) //gown: iso
	go func() {
		<-ch
	}()
	a := &payload{dirt: "lots"} //gown: new
	ch <- a

	// having consumed a, this should be not typecheck:
	println(a)
}
