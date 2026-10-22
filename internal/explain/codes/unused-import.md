# unused-import

An import is never used.

An import is used when the file refers to the package. It is also used when the package has observers or invariants, because importing it turns them on. Otherwise remove the `use` line.

## Example

### p/p.synod
```synod
# package p
use std/lists

define One
  result 1
```

## Fix

Remove the import.

### p/p.synod
```synod
# package p
define One
  result 1
```
