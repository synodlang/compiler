# context

A statement is not allowed in the context it runs in.

Every statement runs in one of four contexts: node, main, check and look (`spec/05` section 5.4). Each context has its own list of what it may read, write and use. A receiver phrase is node code, so it cannot be called from `define main`. Main reaches node state through `p.Node` values instead.

## Example

### p/p.synod
```synod
# package p
Node has
  set Count to 0

define [n: Node] Bump
  set n.Count to (n.Count plus 1)
```

### p/main.synod
```synod
# package main
use example.com/explain/p
use std/sim

define main
  set nodes to (sim.Cluster of 2 running p)
  each n in nodes
    n.p.Bump
```

## Fix

Write the setup in main with a phrase that takes the node as a hole.

### p/p.synod
```synod
# package p
Node has
  set Count to 0

define Bump [n: Node]
  set n.Count to (n.Count plus 1)
```

### p/main.synod
```synod
# package main
use example.com/explain/p
use std/sim

define main
  set nodes to (sim.Cluster of 2 running p)
  each n in nodes
    p.Bump n
```
