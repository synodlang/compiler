# color-literal

A color literal stands where only a named color is allowed.

A color such as `#123456` is allowed only as the `result` of a phrase with no holes, which gives the color a name. Everywhere else write the name, such as `theme.Accent`.

## Example

### p/p.synod
```synod
# package p
define Sky [n: number]
  result #123456
```

## Fix

Remove the holes, so the phrase names the color.

### p/p.synod
```synod
# package p
define Sky
  result #123456
```
