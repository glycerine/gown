package main

import (
	"fmt"
)

//\\observer fmt.Printf

type bigTree struct {
	name string

	// Pretend this is the root of a big tree full of state.
	// We just skip showing all the other stuff.
	// ...
}

func (t *bigTree) clone() *bigTree {
	return &bigTree{
		name: t.name,
	}
}

type ticket struct {
	tree *bigTree //gown:iso

	outcome string
	done    chan *ticket //gown:imm; elem iso
}

func (t *ticket) clone() *ticket {
	return &ticket{
		tree:    t.tree.clone(),
		outcome: t.outcome,
		done:    make(chan *ticket),
	}
}

func newTicket(name string) *ticket { //gown:iso
	return &ticket{
		tree: &bigTree{
			name: name,
		},
		done: make(chan *ticket), //gown:iso
	}
}

type worker struct {
	getJob chan *ticket //gown: elem iso
	end    chan struct{}
}

func newWorker() *worker {
	return &worker{
		getJob: make(chan *ticket), //gown: elem iso
		end:    make(chan struct{}),
	}
}

func (w *worker) runBackgrounWorkerGoro() {
	go func() {
		for {
			select {
			case tkt := <-w.getJob:
				fmt.Printf("processing tkt.tree.name: '%v'\n", tkt.tree.name)
				tkt.outcome = "ok"
				tkt.done <- tkt
				// even though we own tkt, it is still
				// illegal to change the \imm done field.
				// The submitter is depending on hearing
				// back on that particular channel
				//tkt.done = nil // GWN005: cannot write through \imm value "tkt"
				//tkt.done = make(chan \iso *ticket) // ditto; same GWN005 error
			case <-w.end:
				return
			}
		}
	}()
}

// main "supervises" a worker goroutine, issuing a job tickets to it
// and waiting for the worker to send back the ticket on the done channel.
func main() {
	w := newWorker()
	w.runBackgrounWorkerGoro()
	defer close(w.end) // tell the worker to exit when we do.

	tkt := newTicket("ticket_0")

	demonstrateClone := false

	if demonstrateClone {
		tkt2 := (tkt).clone() //gown:clone
		w.getJob <- tkt2
		// still be legal to touch tkt now because we only cloned it.
		fmt.Printf("tkt is: '%#v'\n", tkt)
		// but it is illegal to touch tkt2 now, since the worker now owns it.
		//fmt.Printf("tkt2 is: '%#v'\n", \unsafe(tkt2))
		tkt = <-tkt2.done
	} else {
		w.getJob <- tkt
		// illegal to touch tkt now that we moved ownership to the worker.
		//fmt.Printf("tkt is: '%#v'\n", tkt)
		// except to read an \imm channel or rebind it:
		//tkt = <-tkt.done // okay
		// also okay:
		tkt3 := <-tkt.done
		tkt = tkt3
		tkt3 = nil // gown added: nil out because ownership transferred
	}

	fmt.Printf("tkt.tree.name = '%v'\n", tkt.tree.name)
	fmt.Printf("got <-tkt.done: tkt.outcome = '%v'\n", tkt.outcome)
}
