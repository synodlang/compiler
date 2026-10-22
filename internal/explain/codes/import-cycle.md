# import-cycle

Packages import each other in a circle.

Imports must form a graph without cycles, because a node that runs a package also runs everything it uses, in dependency order. The error shows the cycle, for example `a → b → a`. Move what both need into a third package.

## Example

### a/a.synod
```synod
# package a
use example.com/explain/b

define One
  result b.Two
```

### b/b.synod
```synod
# package b
use example.com/explain/a

define Two
  result a.One
```

## Fix

Break the cycle so that only one package imports the other.

### b/b.synod
```synod
# package b
define Two
  result 2
```
