# reserved-word

A reserved word is used as a name or a head.

Words like `to`, `if`, `list` and `by` belong to the language, so they cannot name a field, a record type, a phrase head or a variable. The full list is in `spec/01`. Reserved words may appear as later words of a pattern, such as `Send [m: msg] to [n: node]`.

## Example

### p/p.synod
```synod
# package p
Node has
  set list to 0
```

## Fix

Choose a name that is not a reserved word.

### p/p.synod
```synod
# package p
Node has
  set items to 0
```
