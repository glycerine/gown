package gown

import "testing"

const gownSSAInventorySource = `package example

type payload struct {
	Data string
}

type inner struct {
	Item *payload
}

type holder struct {
	Inner inner
	Items []*payload
	M map[string]*payload
	Value *payload
}

func Use(x *payload) *payload {
	return x
}

func Consume(x *payload) {}

func choose(cond bool, a, b *payload) *payload {
	x := a
	if cond {
		x = b
	}
	return x
}

func main() {
	h := &holder{}
	ch := make(chan *payload, 1)
	i := 0

	h.Inner.Item = Use(h.Inner.Item)
	_ = h.Inner.Item
	_ = &h.Inner.Item
	_ = h.Items[i]
	_ = h.M["k"]
	var any interface{} = h.Inner.Item
	_ = any

	go Consume(h.Inner.Item)
	go func() {
		Consume(h.Inner.Item)
	}()

	ch <- choose(true, h.Inner.Item, h.Value)
}
`

func TestSSAInventoryRecordsInstructionKinds(t *testing.T) {
	facts := loadSSAInventoryFacts(t)

	for _, want := range []string{
		"Call",
		"FieldAddr",
		"Go",
		"IndexAddr",
		"Lookup",
		"MakeClosure",
		"MakeInterface",
		"Send",
		"Store",
		"UnOp",
	} {
		if !ssaInventoryHas(facts, "main", want) {
			t.Fatalf("SSA inventory missing %s in main; facts:\n%s", want, formatSSAInventoryFacts(facts))
		}
	}
	if !ssaInventoryHas(facts, "choose", "Phi") {
		t.Fatalf("SSA inventory missing Phi in choose; facts:\n%s", formatSSAInventoryFacts(facts))
	}
}

func TestSSAInventoryFactsCarrySourceLines(t *testing.T) {
	facts := loadSSAInventoryFacts(t)

	send, ok := firstSSAInventoryFact(facts, "main", "Send")
	if !ok {
		t.Fatalf("SSA inventory missing Send; facts:\n%s", formatSSAInventoryFacts(facts))
	}
	if send.Line != 50 {
		t.Fatalf("Send line = %d, want 50; fact: %#v", send.Line, send)
	}

	goFact, ok := firstSSAInventoryFact(facts, "main", "Go")
	if !ok {
		t.Fatalf("SSA inventory missing Go; facts:\n%s", formatSSAInventoryFacts(facts))
	}
	if goFact.Line != 45 {
		t.Fatalf("first Go line = %d, want 45; fact: %#v", goFact.Line, goFact)
	}
}

func loadSSAInventoryFacts(t *testing.T) []ssaInventoryFact {
	t.Helper()
	dir := writeGownDir(t, map[string]string{"inventory.gown": gownSSAInventorySource})
	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	return collectSSAInventoryFacts(gp.pkg, gp.ssaPkg)
}
