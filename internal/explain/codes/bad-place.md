# bad-place

`set` targets something that cannot be set.

You can set a local, a node field, or an element of one with `set item … of`. Hole parameters, `when` bindings, `each` names and record fields cannot be set. A local that was made outside an `after` block or a block argument cannot be set inside it, because the block holds a copy.

## Example

### p/p.synod
```synod
# package p
define Next [k: number]
  set k to (k plus 1)
  result k
```

## Fix

Set a local instead of the parameter.

### p/p.synod
```synod
# package p
define Next [k: number]
  set next to (k plus 1)
  result next
```
