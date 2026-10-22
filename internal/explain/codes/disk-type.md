# disk-type

A value read from a node's disk does not have the type of the default given to `disk.Read`.

`disk.Read k on n or d` returns the value stored under `k`, or `d` when nothing is stored. The default fixes the type of the result, so what is stored has to be accepted as that type. A list stored where a map is expected is not.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use example.com/explain/p
use std/sim

define main
  set a to (sim.Spawn p)
  after 1s
    sim.Remove a
```

### p/p.synod
```synod
# package p
use std/disk

Node has
  set Pages to {}

define [n: Node] start
  disk.Write {"a"} at "pages" on n
  set n.Pages to (disk.Read "pages" on n or {})
  set item "k" of n.Pages to "v"
```

## Fix

Write the same kind of value that the default has.

### p/p.synod
```synod
# package p
use std/disk

Node has
  set Pages to {}

define [n: Node] start
  disk.Write {"a": "b"} at "pages" on n
  set n.Pages to (disk.Read "pages" on n or {})
  set item "k" of n.Pages to "v"
```
