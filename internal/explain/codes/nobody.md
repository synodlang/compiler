# nobody

`sim.Nobody` was used where a node is needed.

`sim.Nobody` stands for no node at all. It is fine as a value to keep or to compare, but sending to it, crashing, restarting or removing it, naming it in `net.Partition` or `net.DropNext`, delivering to it, or reading a node field of it has nothing to act on.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.Crash sim.Nobody
```

## Fix

Name a real node, such as one made with `sim.Spawn`.

### p/main.synod
```synod
# package main
use example.com/explain/p
use std/sim

define main
  set a to (sim.Spawn p)
  sim.Crash a
```

### p/p.synod
```synod
# package p
Node has
  set Ready to false
```
