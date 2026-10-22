# unexported

A lower-case name from another package is used.

Capitalization has one meaning: exported. Only names that start upper-case (phrase heads, record types, record fields and node fields) can be used outside the package that declares them. To share a name, spell it upper-case where it is declared.

## Example

### p/p.synod
```synod
# package p
use example.com/explain/q

define Go
  result (q.secret 1)
```

### q/q.synod
```synod
# package q
define secret [n: number]
  result n
```

## Fix

Export the phrase by giving it an upper-case head.

### p/p.synod
```synod
# package p
use example.com/explain/q

define Go
  result (q.Secret 1)
```

### q/q.synod
```synod
# package q
define Secret [n: number]
  result n
```
