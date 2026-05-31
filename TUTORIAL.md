# Go + Ownership = Gown. the Gown tutorial

The Gown pre-processor allows the Go developer
to describe pointer ownership and data immutability, and thus
catches data-races in Go code at compile time.

## overview

Inspired a little by Rust, and alot by Pony's 
capability-ownership system, Gown is a pre-processor 
for Go source that statically detects use-after-move
data-races at compile time. 

Gown is much simpler than Pony. Gown is also 
much, much simpler than Rust. Rust requires lifetime
annotations, Gown does not. Pony has six capability
annotations. Gown has only four.

In one line, the summary of Gown would be: 
channel sends can now enforce at compile time the former 
"convention only" transfer of ownership.

The most common data race in my own Go programs is simultaneous access after
I have sent data to another goroutine over a channel. 

Gown aims to catch that mistake early, long before runtime.

To do this Gown analyzes the SSA form of a Go package. It
will conservatively reject programs that it cannot prove
correct. Thus some re-arrangement of pointer manipulation,
aiming for provable safety, may be required, particularly
after a select{} statement that sends an \iso pointer. To my thinking,
this is a small inconvenience in exchange for data-race freedom.

## introduction

In Gown there are only four core annotations
on pointers: \iso for single-owner (isolated) mutable data, \imm for
immutable data, \mub for mutable borrow, and \rob for read-only borrowed data.

The annotations are also called capabilities. There
are also some helpers like \new and \clone which create new \iso
pointers that we will get to later in this tutorial. For now we concentrate
on the capability definitions.

Each capability tells the Gown checker what kind of 
access a piece of code has to a value: unique ownership (\iso),
local mutation (\mub), read-only access (\rob), or 
immutable and thus safe for sharing (\imm)

As a pre-processor, Gown aims to check for data-races before Go
compilation starts. You write `.gown` files, Gown checks them, 
and then Gown emits ordinary `.go` files with the annotations erased.

This tutorial is an introduction and starting point. For the 
full formal reference, see `gown-spec.md`. The formal proofs
of soundness are in the theory-proof.md and theory-proof-gemini-v2.md
files, which are backed by a `Gown.lean` LEAN proof.

### the core idea

In ordinary Go, a pointer does not say much about who else might hold the same
pointer. That is flexible, but it can make concurrent programs hard to reason
about.

Gown lets you write down the important aliasing promise:

```go
type Buffer struct {
    Data []byte
}
```

If a value has type `\iso *Buffer`, then there is one isolated owner. If a value
has type `\imm *Buffer`, then it is deeply immutable and safe to share. If a
value has type `\mub *Buffer` or `\rob *Buffer`, then it is a local borrow that
must not cross goroutine boundaries.

The four core annotations are:

| Annotation | Name | Mutable? | Sendable across goroutines? | Main idea |
| --- | --- | --- | --- | --- |
| `\iso` | isolated | yes | yes, by move | one unique owner |
| `\mub` | mutable borrow | yes | no | temporary local mutation |
| `\rob` | read-only borrow | no | no | temporary local reading |
| `\imm` | immutable | no | yes, by sharing | safe to share freely |

## annotation syntax

Capability annotations appear before the `*` in pointer types:

```go
func Take(b \iso *Buffer) {}
func Mutate(b \mub *Buffer) {}
func Inspect(b \rob *Buffer) {}
func Share(b \imm *Buffer) {}
```

They can appear on function parameters, function return types, struct fields,
interface methods, channel element types, and explicit local variable
declarations.

Local variables are usually inferred from the right-hand side:

```go
b := \new(Buffer{})
```

Here `b` is inferred as `\iso *Buffer`.

All Gown annotations begin with `\`. If an annotation accidentally leaks into
generated Go, the Go compiler will reject it. That makes annotation leakage
fail fast instead of silently changing the program.

## `\iso`: isolated ownership

Use `\iso` when one part of the program owns a mutable value uniquely.

An `\iso` value may be read and written. It may also be sent to another
goroutine, but sending it is a move: after the move, the sender no longer owns
the value.

```go
type Buffer struct {
    Data []byte
}

func Take(b \iso *Buffer) {
    b.Data = append(b.Data, 1)
}

func main() {
    b := \new(Buffer{})
    Take(b)

    // Error: b was moved into Take.
    _ = b
}
```

Passing an `\iso` to a parameter declared `\iso` transfers ownership to the
callee. The caller must stop using the old variable.

### rebinding after a move

A moved variable can be used again after it is assigned a fresh valid value.

```go
func main() {
    b := \new(Buffer{})
    Take(b)

    b = \new(Buffer{})
    b.Data = append(b.Data, 2) // ok: b has been rebound
}
```

Rebinding is common. The important rule is that the new value must itself be a
valid `\iso` value.

### assignment moves ownership

Assigning one `\iso` variable to another is also a move.

```go
func main() {
    a := \new(Buffer{})
    b := a

    b.Data = append(b.Data, 1) // ok

    // Error: a was moved into b.
    _ = a
}
```

### sending `\iso` on a channel

An `\iso` value can be sent across a channel whose element type is `\iso`.

```go
func main() {
    ch := make(chan \iso *Buffer)

    b := \new(Buffer{})
    ch <- b

    // Error: b was moved into the channel.
    _ = b
}
```

The receiver becomes the new owner:

```go
func worker(ch chan \iso *Buffer) {
    b := <-ch
    b.Data = append(b.Data, 1)
    Take(b)
}
```

## `\mub`: mutable borrow

Use `\mub` when a function needs to mutate a value temporarily, but should not
take ownership of it. For example, a function that receives a \mub parameter could not send it to another goroutine over an \iso channel. In this example, AppendByte can mutate b, but not give it away. We know after AppendByte returns that main still has ownership of b. AppendBytes cannot store the pointer for later. AppendBytes cannot \freeze the pointer making it immutable.

```go
func AppendByte(b \mub *Buffer, x byte) {
    b.Data = append(b.Data, x)
}

func main() {
    b := \new(Buffer{})

    AppendByte(b, 7)
    AppendByte(b, 8)

    // Still ok: AppendByte only borrowed b.
    Take(b)
}
```

The caller keeps its `\iso` after passing it to a `\mub` parameter. Gown inserts
an implicit mutable borrow for the duration of the call.

You can also create an explicit named mutable borrow:

```go
func main() {
    b := \new(Buffer{})

    mb := \mub(b)
    mb.Data = append(mb.Data, 1)
    mb.Data = append(mb.Data, 2)

    // Once mb is no longer live, b can be moved.
    Take(b)
}
```

`\mub` is goroutine-local. It cannot be sent over a channel.

```go
func Bad(ch chan \iso *Buffer, b \mub *Buffer) {
    ch <- b // error: mutable borrows are not sendable
}
```

Use `\mub` for ordinary local mutation. It is the annotation that most closely
matches normal single-goroutine Go pointer use.

## `\rob`: read-only borrow

Use `\rob` when a function should be allowed to inspect a value but not mutate
it. It allows you to write functions that can process any kind of data, be it immutable or mutable or isolated.

```go
func Len(b \rob *Buffer) int {
    return len(b.Data)
}

func main() {
    b := \new(Buffer{})

    n := Len(b)
    _ = n

    // Still ok: Len only borrowed b.
    Take(b)
}
```

The caller keeps its ownership. The read-only borrow lasts for the call.

A `\rob` value cannot be used to write:

```go
func Bad(b \rob *Buffer) {
    b.Data = nil // error: cannot write through a read-only borrow
}
```

You can create an explicit named read-only borrow when you want to use the
borrow across more than one expression:

```go
func main() {
    b := \new(Buffer{})

    rb := \rob(b)
    _ = len(rb.Data)
    _ = cap(rb.Data)

    Take(b)
}
```

`\rob` is also goroutine-local. It is read-only, but it is still a borrow, not
a shareable value. Use `\imm` when you want safe sharing across goroutines.

## `\imm`: deeply immutable (always sharable)

Use `\imm` when many parts of the program may share the same value, including
different goroutines, and none of them may mutate it.

An `\imm` value is deeply immutable: the object and the reachable object graph
behind it must not be mutated through the immutable reference.

You usually create `\imm` by freezing an `\iso`:

```go
func ReadOnlyLen(b \imm *Buffer) int {
    return len(b.Data)
}

func main() {
    b := \new(Buffer{})
    shared := \freeze(b)

    _ = ReadOnlyLen(shared)
    _ = ReadOnlyLen(shared) // ok: imm values can be reused

    // Error: b was consumed by freeze.
    _ = b
}
```

Unlike `\iso`, sending an `\imm` value does not consume it.

```go
func main() {
    ch := make(chan \imm *Buffer)

    b := \new(Buffer{})
    shared := \freeze(b)

    ch <- shared
    ch <- shared // ok: immutable values are shareable

    _ = shared // still ok
}
```

Writes through `\imm` are rejected:

```go
func Bad(b \imm *Buffer) {
    b.Data = nil // error: cannot write through immutable data
}
```

## built-ins

Gown has a small set of built-in operations. They look like function calls, but
they are preprocessor constructs.

| Operation | Meaning | Output idea |
| --- | --- | --- |
| `\new(T{...})` | allocate a fresh isolated value | `&T{...}` |
| `\clone(x)` | call a trusted same-type `Clone` method | `(x).Clone()` |
| `\freeze(x)` | consume `\iso`, produce `\imm` | assignment plus consumed source |
| `\mub(x)` | make an explicit mutable borrow | `x` |
| `\rob(x)` | make an explicit read-only borrow | `x` |
| `\unsafe(x)` | cross an unchecked boundary explicitly | `x` |

### `\new`

Use `\new` to create a fresh `\iso` pointer from a struct literal.

```go
func main() {
    b := \new(Buffer{Data: []byte{1, 2, 3}})
    Take(b)
}
```

### `\clone`

Use `\clone` when you want a fresh isolated copy while keeping the original
available.

For a value of static type `T`, `T` must have:

```go
Clone() T
```

For a value of static type `*T`, `*T` must have:

```go
Clone() *T
```

Example:

```go
func (b *Buffer) Clone() *Buffer {
    cp := *b
    cp.Data = append([]byte{}, b.Data...)
    return &cp
}

func main() {
    template := \new(Buffer{Data: []byte{1, 2, 3}})

    copy1 := \clone(template)
    copy2 := \clone(template)

    Take(copy1)
    Take(copy2)

    // template was not consumed by clone.
    Take(template)
}
```

Gown trusts `Clone` to return an independent value. The checker verifies the
method shape, but it cannot prove that the method body performed a deep copy.

### `\freeze`

Use `\freeze` when you are done mutating an isolated value and want to share it.

```go
func Publish(ch chan \imm *Buffer, b \iso *Buffer) {
    shared := \freeze(b)
    ch <- shared
    ch <- shared
}
```

Freezing consumes the original `\iso`.

### `\unsafe`

Use `\unsafe` only at an explicit checked-to-unchecked boundary, such as a call
to ordinary Go code that Gown cannot analyze.

```go
func LegacyUse(b *Buffer) {
    // ordinary Go code
}

func main() {
    b := \new(Buffer{})

    LegacyUse(\unsafe(b))

    // Later capability operations on b may be rejected, because the checker no
    // longer has a complete proof of what happened beyond the unsafe boundary.
}
```

`\unsafe` is intentionally visible and searchable. It is the place where the
programmer says, "I know something the checker cannot verify."

## function patterns

A useful way to learn Gown is to compare the same API shape under different
annotations.

```go
func Own(b \iso *Buffer) {
    // Takes ownership. Caller loses b.
}

func Mutate(b \mub *Buffer) {
    // Temporarily mutates. Caller keeps b.
    b.Data = append(b.Data, 1)
}

func Inspect(b \rob *Buffer) int {
    // Temporarily reads. Caller keeps b.
    return len(b.Data)
}

func Share(b \imm *Buffer) int {
    // Reads a shareable immutable value.
    return len(b.Data)
}
```

At a call site:

```go
func main() {
    b := \new(Buffer{})

    Mutate(b)  // implicit \mub borrow
    Inspect(b) // implicit \rob borrow

    frozen := \freeze(b)
    Share(frozen)
    Share(frozen)
}
```

The key distinction:

| Callee wants | Caller effect |
| --- | --- |
| `\iso` | caller gives up ownership |
| `\mub` | caller lends mutable access temporarily |
| `\rob` | caller lends read-only access temporarily |
| `\imm` | caller shares immutable access |

## channel patterns

Channels are where the difference between moving and sharing becomes very
important.

### Ownership Transfer Channel

Use `chan \iso *T` when each item has one owner at a time.

```go
func producer(ch chan \iso *Buffer) {
    b := \new(Buffer{})
    ch <- b

    // b is gone here.
}

func consumer(ch chan \iso *Buffer) {
    b := <-ch
    b.Data = append(b.Data, 1)
    Take(b)
}
```

### immutable broadcast channel

Use `chan \imm *T` when many receivers may safely share the same data.

```go
func publish(ch chan \imm *Buffer) {
    b := \new(Buffer{Data: []byte{1, 2, 3}})
    shared := \freeze(b)

    ch <- shared
    ch <- shared
    ch <- shared
}
```

### sending a clone

When you want to keep a local value but send a fresh owned copy, clone at the
send site.

```go
func publishCopies(ch chan \iso *Buffer, template \rob *Buffer) {
    ch <- \clone(template)
    ch <- \clone(template)
}
```

The channel receives fresh `\iso` values. The template is not consumed.

### stable reply channels

Sometimes an owned value needs to carry a reply channel with it. The worker gets
ownership of the value, does some work, and then sends the value back.

The reply channel field should be declared `\imm`:

```go
type Ticket struct {
    Data string
    Done \imm chan \iso *Ticket
}

func NewTicket(data string) \iso *Ticket {
    return &Ticket{
        Data: data,
        Done: make(chan \iso *Ticket),
    }
}
```

The `\imm` annotation says the channel handle stored in `Done` is stable. The
field may be read even after the parent ticket has moved, because nobody is
allowed to replace the channel handle.

```go
func main(work chan \iso *Ticket) {
    t := NewTicket("demo")

    work <- t

    // t moved into work, but t.Done is a stable immutable field.
    t = <-t.Done

    println(t.Data)
}
```

Without `\imm`, this would be unsafe:

```go
type BadTicket struct {
    Done chan \iso *BadTicket
}
```

After `BadTicket` moves to another goroutine, the new owner could reassign
`Done` at the same time the old owner tries to read it. That would be a race on
the field slot. `\imm chan ...` prevents that by making the field slot
read-only.

## struct fields

Struct fields may also carry capability annotations.

```go
type Job struct {
    Input  \iso *Buffer
    Config \imm *Buffer
    Done   \imm chan \iso *Job
}
```

If you own a `\iso *Job`, then moving `job.Input` out directly is restricted:
moving from a field projection can leave the containing object partially moved.
The checker rejects unsafe field moves.

A common pattern is to expose operations as methods or functions that preserve
the ownership story:

```go
func UseConfig(j \rob *Job) int {
    return len(j.Config.Data)
}

func ReplaceInput(j \mub *Job, next \iso *Buffer) {
    j.Input = next
}
```

For beginners, it is enough to remember:

- Put capabilities on fields that store tracked pointers.
- Read-only or immutable access through the outer object makes reachable fields
  read-only too.
- Use `\imm chan ...` for stable channel fields that must be read after the
  parent object moves.
- Be careful moving ownership out of fields; prefer explicit helper functions.

## common errors

Gown reports structured errors with codes. These are the ones beginners usually
hit first.

| Code | Meaning | Common fix |
| --- | --- | --- |
| `GWN001` | used an `\iso` after it was moved or consumed | rebind it, clone before moving, or stop using the old variable |
| `GWN002` | a live borrow blocks a move or freeze | shorten the borrow lifetime |
| `GWN003` | tried to send `\mub` or `\rob` across a channel | send `\iso` or `\imm` instead |
| `GWN005` | wrote through `\rob` or `\imm` | use `\mub` or keep unique `\iso` ownership |
| `GWN010` | invalid capability conversion, channel mismatch, or clone shape | adjust the annotation or method signature |
| `GWN012` | tried to use a value as tracked after a proof frontier | keep it tracked or use an explicit unsafe boundary |

## a small complete example

This example uses ownership transfer for work items and immutable sharing for
configuration.

```go
package main

type Config struct {
    Prefix string
}

type Request struct {
    Body []byte
}

func (r *Request) Clone() *Request {
    cp := *r
    cp.Body = append([]byte{}, r.Body...)
    return &cp
}

func Normalize(r \mub *Request) {
    r.Body = append([]byte("normalized:"), r.Body...)
}

func Size(r \rob *Request) int {
    return len(r.Body)
}

func Handle(cfg \imm *Config, r \iso *Request) {
    Normalize(r)
    _ = cfg.Prefix
    _ = Size(r)

    // r is still owned here because Normalize and Size borrowed it.
    Finish(r)
}

func Finish(r \iso *Request) {
    _ = r.Body
}

func main() {
    work := make(chan \iso *Request, 2)

    cfgIso := \new(Config{Prefix: "demo"})
    cfg := \freeze(cfgIso)

    template := \new(Request{Body: []byte("hello")})

    work <- \clone(template)
    work <- \clone(template)

    first := <-work
    Handle(cfg, first)

    second := <-work
    Handle(cfg, second)

    // template was never consumed by the clone sends.
    Finish(template)
}
```

What happened here:

- `cfgIso` started as mutable unique data.
- `cfg := \freeze(cfgIso)` made the config immutable and shareable.
- `template` stayed locally owned.
- `\clone(template)` created fresh isolated requests for the channel.
- `Normalize` borrowed each request mutably.
- `Size` borrowed each request read-only.
- `Finish` consumed each request.

## exercises

Try these changes in small `.gown` files:

1. Write a function that takes `\iso *Buffer` and prove to yourself that the
   caller cannot use the old variable afterward.
2. Change that function to take `\mub *Buffer` and observe that the caller keeps
   ownership.
3. Add a read-only helper with `\rob *Buffer`, then try to write through the
   parameter and see the checker reject it.
4. Freeze an `\iso *Buffer` into `\imm *Buffer`, send it twice on a channel, and
   confirm the sender still has the immutable value.
5. Add a valid `Clone() *Buffer` method, then send `\clone(b)` while continuing
   to use `b`.
6. Try to send a `\mub *Buffer` on a channel and explain why Gown rejects it.
7. Add a `Done \imm chan \iso *Ticket` reply channel to a ticket type, send the
   ticket to a worker, and receive the ticket back from `Done`.

## quick reference

Use `\iso` when there is exactly one mutable owner.

Use `\mub` when code needs temporary local mutation without taking ownership.

Use `\rob` when code needs temporary local read-only access.

Use `\imm` when data should be deeply immutable and freely shareable.

Use `\imm chan \iso *T` when a struct field stores a stable reply channel whose
handle must be read after the parent value moves.

Use `\new` to create fresh isolated values.

Use `\clone` to make trusted fresh isolated copies.

Use `\freeze` to turn isolated mutable data into immutable shared data.

Use `\unsafe` only when deliberately crossing into unchecked Go code.
