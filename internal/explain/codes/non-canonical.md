# non-canonical

The code can be read but is not written in canonical form.

Synod has one spelling for each meaning. If a file is parseable but spelled another way, the compiler reports `non-canonical`. Examples are `require` lines that are not sorted, settings of `message … with` out of field order, a name that must start lower-case, CR LF line endings, and a package's own qualifier used inside the package, and `use std/theme/default` in a main file (the default theme needs no import).

You do not fix these by hand. Run `synod fmt` and it rewrites the file into canonical form.

## Example

### p/p.synod
```synod
# package p
Promise has
  set number to 0
  set prior to 0

define Make
  result (message Promise with prior 1, number 2)
```

## Fix

Write the settings in the order the fields are declared, or let `synod fmt` do it.

### p/p.synod
```synod
# package p
Promise has
  set number to 0
  set prior to 0

define Make
  result (message Promise with number 2, prior 1)
```
