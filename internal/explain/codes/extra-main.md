# extra-main

A folder has more than one `define main`, or `define main` is in a library file.

A folder holds at most one main file, a main file holds exactly one `define main`, and `define main` appears only in main files.

## Example

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.Seed 1

define main
  sim.Seed 2
```

## Fix

Keep one `define main`.

### p/main.synod
```synod
# package main
use std/sim

define main
  sim.Seed 1
```
