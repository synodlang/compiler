# unused-name

A local or `each` name is never read.

Like Go, Synod rejects an unused local. A name is used when it is read, or when it is the base of `set item … of x`. Setting it again is not a use. Hole parameters and names bound by `when` may stay unused.

## Example

### p/p.synod
```synod
# package p
Node has
  set count to 0

define [n: Node] Bump
  set step to 1
  set n.count to (n.count plus 1)
```

## Fix

Use the local, or remove it.

### p/p.synod
```synod
# package p
Node has
  set count to 0

define [n: Node] Bump
  set step to 1
  set n.count to (n.count plus step)
```
