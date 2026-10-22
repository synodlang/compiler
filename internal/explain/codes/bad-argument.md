# bad-argument

An argument is outside the domain of the phrase that received it.

Many phrases accept only part of their type. A delay must not be negative, `net.Loss` takes a chance from 0 to 1, `net.Latency` needs `0 ≤ a ≤ b`, `random.Between` needs `b` not below `a`, and `sim.Seed` needs a whole number from 0 to 2^53. The message names the phrase and the value it refused.

In a look the same code covers a bad `Tween` property, a negative size, a `Place` cycle and similar mistakes in drawing.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use std/sim

define main
  after (0s minus 1s)
    sim.StopAt 1s
```

## Fix

Pass a value inside the domain, here a delay of 0 or more.

### p/main.synod
```synod
# package main
use std/sim

define main
  after 1s
    sim.StopAt 1s
```
