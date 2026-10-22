# run-limit

The run exceeded one of the limits that keep a program from running forever.

The limits are 100000 steps at one instant of simulated time, a call depth of 500, and 1000000 items in a list or in a text. A phrase that calls itself without an end, or a timer that fires at the same instant again and again (`time.Every 0s`), hits one of them. The message says which.

## Example

Run with `synod run p`.

### p/main.synod
```synod
# package main
use example.com/explain/p

define main
  p.Down
```

### p/p.synod
```synod
# package p
define Down
  Down
```

## Fix

Let the call end, here by taking a count that runs out.

### p/p.synod
```synod
# package p
define Down [n: number]
  if (n more than 0)
    Down (n minus 1)
```

### p/main.synod
```synod
# package main
use example.com/explain/p

define main
  p.Down 3
```
