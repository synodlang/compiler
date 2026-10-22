# index-range

A list index is not a whole number from 0 up to one less than the size of the list.

Lists count from 0, so a list of 3 items has the indexes 0, 1 and 2. `item i of xs` and `set item i of xs to v` need a whole number in that range. `random.Pick` of an empty list has no item to pick and raises the same code.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use std/sim

define main
  set xs to {1s, 2s, 3s}
  sim.StopAt (item 3 of xs)
```

## Fix

Use an index below the size of the list, counting from 0.

### p/main.synod
```synod
# package main
use std/sim

define main
  set xs to {1s, 2s, 3s}
  sim.StopAt (item 2 of xs)
```
