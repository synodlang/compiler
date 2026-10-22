# time-limit

The next event is later than 10 minutes of simulated time.

A run ends when nothing is left to do, when it is stopped, or when the clock would pass 10 minutes of simulated time. Reaching that limit is an error, not a normal end, so a program that waits for something that never comes does not look like it succeeded. The error has no position, because no single line caused it.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use std/sim

define main
  after 601s
    sim.StopAt 1s
```

## Fix

Keep every delay within 10 minutes.

### p/main.synod
```synod
# package main
use std/sim

define main
  after 60s
    sim.StopAt 1s
```
