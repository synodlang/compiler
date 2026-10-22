# duplicate-name

A name is defined twice in one namespace.

All files of a package share one namespace for phrases, record types, the node type and node fields. A phrase's name is its word skeleton, so there is no overloading by hole types or by the number of holes. The same error covers a second `start`, a repeated hole name, two imports of one path, and a field set twice in `message … with`.

## Example

### p/p.synod
```synod
# package p
define Total [a: number] and [b: number]
  result (a plus b)

define Total [c: number] and [d: number]
  result (c plus d)
```

## Fix

Give the second phrase another name.

### p/p.synod
```synod
# package p
define Total [a: number] and [b: number]
  result (a plus b)

define Sum [c: number] and [d: number]
  result (c plus d)
```
