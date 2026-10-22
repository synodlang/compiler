# bad-header

The file's package header is missing or wrong.

The first non-blank line of every file is `# package <name>`. The name is the folder name, or `main` for the folder's one main file. It is also an error if the name starts upper-case or is a reserved word.

## Example

### p/p.synod
```synod
# package q
define One
  result 1
```

## Fix

Name the package after its folder.

### p/p.synod
```synod
# package p
define One
  result 1
```
