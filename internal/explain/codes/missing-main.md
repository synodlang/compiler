# missing-main

A main file has no `define main`, or `synod run` finds no main file.

A main file is the program of its folder, and it holds exactly one `define main`. `synod run` on a folder without a main file reports the same code.

## Example

### p/main.synod
```synod
# package main
```

## Fix

Add the entry.

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.Seed 1
```
