# bad-receiver

A receiver is of the wrong type or in the wrong place.

A receiver has type `Node` or `p.Node`, and the receiver of `start` and of `when` is `Node`. A `when` on a node event needs a receiver, and a `when` on a trace event must not have one. A main file declares no receivers, no `start`, no node-event `when` and no `Node has`, because it never runs on nodes.

## Example

### p/p.synod
```synod
# package p
define [n: number] Double
  result (n times 2)
```

## Fix

Give the receiver the type `Node`.

### p/p.synod
```synod
# package p
Node has
  set count to 0

define [n: Node] Double
  result (n.count times 2)
```
