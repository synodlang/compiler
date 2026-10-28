# extra-theme

A main file imports more than one theme.

A program has one theme. `std/theme/default` is the theme of a program whose main file imports none, so importing it next to another theme is a second theme. Keep one import.

## Example

### warm/warm.synod
```synod
# theme warm
define Paper
  result #2B1F1A

define Ink
  result #EDE9E1

define Line
  result #3A4366

define Quiet
  result #8A90A8

define Accent
  result #FF7A3D

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

### p/main.synod
```synod
# package main
use example.com/explain/warm
use std/sim
use std/theme/default

define main
  sim.Seed 1
```

## Fix

Remove the second theme.

### p/main.synod
```synod
# package main
use example.com/explain/warm
use std/sim

define main
  sim.Seed 1
```
