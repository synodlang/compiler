# unneeded-alias

`use … as` is written where there is no collision.

An alias is only for two imports of the file that share a last segment, or an import whose last segment is the name of the importing package. Anywhere else the qualifier is the last segment of the path.

## Example

### p/p.synod
```synod
# package p
use example.com/explain/q as qq

define Go
  result qq.One
```

### q/q.synod
```synod
# package q
define One
  result 1
```

## Fix

Remove the alias and use the last segment.

### p/p.synod
```synod
# package p
use example.com/explain/q

define Go
  result q.One
```
