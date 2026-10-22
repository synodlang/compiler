# value-as-statement

A phrase that gives a value stands where a statement is expected.

A value phrase gives its value with `result`. If you write it on a line by itself, the value is thrown away, which is almost always a mistake. Use the value, for example with `set`.

## Example

### p/p.synod
```synod
# package p
Node has
  set count to 0

define Twice [x: number]
  result (x times 2)

define [n: Node] Go
  Twice 3
```

## Fix

Use the value.

### p/p.synod
```synod
# package p
Node has
  set count to 0

define Twice [x: number]
  result (x times 2)

define [n: Node] Go
  set n.count to (Twice 3)
```
