# foreign-state

Node code reaches another node's state.

In node code, state is reached only through the receiver of the `define` or `when` that contains the code. Any other node value gives identity only. Other nodes learn through `net.Send`. The same error covers a node other than the receiver after `from`, `on` or `by`.

## Example

### p/p.synod
```synod
# package p
Node has
  set count to 0

define [n: Node] Copy from [peer: Node]
  set n.count to peer.count
```

## Fix

Take the value as a hole. A real peer would send it in a message with `net.Send`.

### p/p.synod
```synod
# package p
Node has
  set count to 0

define [n: Node] Take [c: number]
  set n.count to c
```
