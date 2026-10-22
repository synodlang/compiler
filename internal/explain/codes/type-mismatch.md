# type-mismatch

A value has a type that is not accepted where it is used.

Once exactly one phrase matches by shape, every hole checks the type of its item. The same check applies to operands, list and map elements, and places that are set again. Types never convert by themselves; use a phrase such as `text.From` to change one. `spec/04` section 4.7 lists which types are accepted as which.

## Example

### p/p.synod
```synod
# package p
Node has
  set count to 0

define Twice [x: number]
  result (x times 2)

define [n: Node] Go
  set n.count to (Twice "five")
```

## Fix

Pass a value of the type the hole expects.

### p/p.synod
```synod
# package p
Node has
  set count to 0

define Twice [x: number]
  result (x times 2)

define [n: Node] Go
  set n.count to (Twice 5)
```
