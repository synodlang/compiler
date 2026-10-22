# unknown-import

An import path does not name a library package.

The path must name a folder that holds a library file. It fails when the folder is missing, when it holds only a main file, when the `std/` package does not exist, when the module is not required or not in `vendor/`, or when the version differs from the selected one. The std module is imported only as `std/<package>`.

## Example

### p/p.synod
```synod
# package p
use example.com/explain/helpr

define Go
  result helpr.One
```

### helper/helper.synod
```synod
# package helper
define One
  result 1
```

## Fix

Spell the path as the folder is named.

### p/p.synod
```synod
# package p
use example.com/explain/helper

define Go
  result helper.One
```
