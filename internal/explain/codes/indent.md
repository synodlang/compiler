# indent

The indentation of a line is wrong.

Indentation is exactly 2 spaces per level. A tab in indentation, an odd width, or an indent where no block opens is an `indent` error. A comment line is not checked, so it never opens or closes a block.

## Example

### p/p.synod
```synod
# package p
define Half
   result 0.5
```

## Fix

Indent each level by exactly 2 spaces.

### p/p.synod
```synod
# package p
define Half
  result 0.5
```
