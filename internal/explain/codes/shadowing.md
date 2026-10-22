# shadowing

A name hides another name that is already in scope.

Every name has one meaning. A local, a binding, a hole or an `each` name must not be spelled like another of these, like a zero-hole phrase, or like a record type of the package. A node field or phrase head must not be spelled like an import qualifier of the file.

## Example

### p/p.synod
```synod
# package p
define Total [x: number]
  set sum to x
  each x in {1, 2}
    set sum to (sum plus x)
  result sum
```

## Fix

Rename the second name.

### p/p.synod
```synod
# package p
define Total [x: number]
  set sum to x
  each y in {1, 2}
    set sum to (sum plus y)
  result sum
```
