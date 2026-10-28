# theme-outside-main

A library file imports a theme.

The theme of a program is the theme its main file imports, or `std/theme/default` when it imports none. Only a main file selects a theme, because a library must look right under every theme. Move the import to the main file, or remove it.

## Example

### p/p.synod
```synod
# package p
use std/theme/default

define One
  result 1
```

## Fix

Remove the import: the main file of the program selects its theme.

### p/p.synod
```synod
# package p
define One
  result 1
```
