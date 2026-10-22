# division-by-zero

A number or a time was divided by zero.

`divided by` needs a divisor that is not 0 and not `0s`. `math.Remainder of a by b` has the same rule for `b`. The run stops at the division, because the result would not be a number.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.StopAt (1s divided by 0)
```

## Fix

Divide by a number that is not zero.

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.StopAt (1s divided by 2)
```
