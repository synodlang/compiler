# syntax

The structure grammar cannot parse the line.

This is the error for text that is not written the way Synod spells it. Examples are a number such as `.5` or `+1`, `x-1` without spaces, a comma outside list items, a continuation line that follows no block, a `#todo` comment, a nested `use`, and a bad line in `synod.mod`.

The formatter cannot fix these, because it cannot tell what you meant. Write the number, text or line the way `spec/01` and `spec/02` describe it.

## Example

### p/p.synod
```synod
# package p
define Half
  result .5
```

## Fix

Write a digit before the point.

### p/p.synod
```synod
# package p
define Half
  result 0.5
```
