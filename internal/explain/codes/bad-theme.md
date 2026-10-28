# bad-theme

A theme file is not a complete theme.

A theme file holds only `define <Name>` lines with one `result` line each, one for each of the 34 names of `std/theme/default`, each value of the name's type and inside its range. A name that is missing or repeated, a name that is not in the table, a phrase with holes, a `use`, and a value of the wrong type or outside its range are all `bad-theme`. Copy `std/theme/default` and change its values.

## Example

### sunrise/sunrise.synod
```synod
# theme sunrise
define Paper
  result #FFF4E0

define Ink
  result #2B2B2B
```

## Fix

Define every name of the theme table.

### sunrise/sunrise.synod
```synod
# theme sunrise
define Paper
  result #FFF4E0

define Ink
  result #2B2B2B

define Line
  result #3A4366

define Quiet
  result #8A90A8

define Accent
  result #F4A340

define Good
  result #5CC8B8

define Warning
  result #E8C35A

define Critical
  result #E86F5A

define Message1
  result #F4A340

define Message2
  result #7FB2F0

define Message3
  result #C79BEA

define Message4
  result #F08FB0

define Vignette
  result 0

define Sketch
  result 0

define Glow
  result 24

define Shadow
  result 0

define StrokeWidth
  result 2

define CornerRadius
  result 12

define NodeShape
  result "card"

define NodeFill
  result "solid"

define LinkOpacity
  result 1

define Easing
  result "smooth"

define Drift
  result 0

define Sans
  result "Inter"

define Mono
  result "JetBrains Mono"

define TitleSize
  result 28

define LabelSize
  result 16

define ValueSize
  result 15

define CaptionSize
  result 20

define TitleWeight
  result 600

define LabelWeight
  result 500

define Tracking
  result 0

define Margin
  result 4

define Fill
  result 1
```
