# missing-key

A map was asked for a key it does not hold.

`item k of m` reads the value stored under the key `k`. The map has to hold that key when the line runs, because Synod has no default value and does not guess one. The run stops at the line that asked.

The same code is raised in a look when `card of n` or `tag of k` names a shape that was never attached to that key.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use std/sim

define main
  set m to {}
  set item "a" of m to 1s
  sim.StopAt (item "b" of m)
```

## Fix

Set the key before reading it.

### p/main.synod
```synod
# package main
use std/sim

define main
  set m to {}
  set item "a" of m to 1s
  set item "b" of m to 2s
  sim.StopAt (item "b" of m)
```
