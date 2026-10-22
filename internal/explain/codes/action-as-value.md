# action-as-value

A phrase that gives no value is used as a value.

An action phrase has no `result`. It cannot be an argument, the right side of `set`, or a condition. Write it as a statement on its own line.

## Example

### p/p.synod
```synod
# package p
Node has
  set count to 0

define [n: Node] Bump
  set n.count to (n.count plus 1)

define [n: Node] Go
  set n.count to n.Bump
```

## Fix

Call the phrase as a statement.

### p/p.synod
```synod
# package p
Node has
  set count to 0

define [n: Node] Bump
  set n.count to (n.count plus 1)

define [n: Node] Go
  n.Bump
```
