# ambiguous-phrase

More than one phrase matches the sentence by its shape.

Exactly one candidate must match. Types never choose between phrases, so a phrase that has the same shape as a language form, such as `[a] and [b]`, makes every use of it ambiguous. The error names both definitions.

Give the phrase a head and words that no language form uses.

## Example

### p/p.synod
```synod
# package p
Node has
  set ok to false

define Foo and [b: bool]
  result b

define [n: Node] Go
  set n.ok to (Foo and true)
```

## Fix

Rename the phrase so its shape is its own.

### p/p.synod
```synod
# package p
Node has
  set ok to false

define Foo with [b: bool]
  result b

define [n: Node] Go
  set n.ok to (Foo with true)
```
