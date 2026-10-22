# bad-pattern

A `define` or `when` pattern breaks the pattern rules.

The rules are in `spec/03` section 3.1. Two inline holes cannot stand next to each other, every hole is typed, block holes come last, and words after the head start lower-case. A head is never directly followed by a comparison operator, and a hole type must name a known type.

## Example

### p/p.synod
```synod
# package p
define Add [a: number] [b: number]
  result (a plus b)
```

## Fix

Put a word between the two holes.

### p/p.synod
```synod
# package p
define Add [a: number] and [b: number]
  result (a plus b)
```
