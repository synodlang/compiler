# ambiguous-empty

The type of `{}` cannot be told.

`{}` is an empty list or an empty map. Its type comes from where it appears: the field or local it is assigned to, the hole it fills, or how that place is used anywhere in the package. If nothing in the package fixes the type, the compiler asks you to.

## Example

### p/p.synod
```synod
# package p
Node has
  set items to {}
```

## Fix

Use the place with a `std/lists` or `std/maps` phrase, so the type is known.

### p/p.synod
```synod
# package p
use std/lists

Node has
  set items to {}

define [n: Node] Add [v: number]
  set n.items to (lists.Append v to n.items)
```
