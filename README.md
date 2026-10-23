# thumbnail
Extract thumbnail candidates from video and chapter files.

Chapter lines contain a title followed by a timestamp (`[[h:]m:]s`) and a
superscript frame remainder, for example:

```text
Three Sisters 0:06 ³⁶ ᵖˡᵃᶜᵉ²
```

The first group of superscript digits is the frame remainder within the second;
the timestamp immediately before it supplies the whole seconds. Later annotation
digits are ignored. Legacy lines beginning with a timestamp and an optional
`(:N)` frame remainder are also supported.
