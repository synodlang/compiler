# native-outside-std

`native` is used outside `std/`.

`native` binds a pattern to a primitive of the runtime, and only packages under `std/` may do that. Write the phrase with a body.

## Example

### p/p.synod
```synod
# package p
define Raw [n: number] native "sim.seed"
```

## Fix

Write the body in Synod.

### p/p.synod
```synod
# package p
define Raw [n: number]
  result n
```
