# thumbnail
Extract thumbnail candidates from video and chapter files.

Chapter lines contain a timestamp (`[[h:]m:]s`) at the beginning, followed by
the title and a superscript frame remainder, for example:

```text
0:06 Three Sisters ³⁶ ᵖˡᵃᶜᵉ²
```

The first group of superscript digits is the frame remainder within the second;
the timestamp at the beginning supplies the whole seconds. `:` and `։` are
accepted as timestamp separators. Later annotation digits are ignored. Older
title-first lines with the timestamp immediately before the frame remainder,
and legacy lines beginning with a timestamp and an optional `(:N)` frame
remainder, are also supported.
