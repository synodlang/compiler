# missing-result

A value phrase has a path that does not reach `result`.

A phrase whose body contains `result` gives a value. Every path through it must end in a `result`, and all results must have one type. Here the `else` branch has no `result`.

## Example

### p/p.synod
```synod
# package p
define Sign [v: number]
  if (v less than 0)
    result "negative"
  else
    if (v is 0)
      result "zero"
```

## Fix

End every path with a `result`.

### p/p.synod
```synod
# package p
define Sign [v: number]
  if (v less than 0)
    result "negative"
  else
    if (v is 0)
      result "zero"
    else
      result "positive"
```
