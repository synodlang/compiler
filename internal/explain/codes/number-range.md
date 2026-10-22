# number-range

A calculation gave a number that is not finite, or a time that does not fit in 64 bits.

Numbers are 64-bit floating point and times are whole microseconds in 64 bits. A result that overflows, such as a very large product, has no value that later steps could use, so the run stops at the calculation.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.StopAt (1000000s times 1000000000000)
```

## Fix

Keep the calculation within the range of the type.

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.StopAt (1000000s times 2)
```
