package gui

// devTest is a single entry in the dev-only Test menu. Running it has a side
// effect on the GUI (typically injecting canned output) rather than returning
// a value.
type devTest struct {
	name string
	run  func()
}

// exhaustiveMarkdown exercises the markdown features we expect the response
// renderer to handle. It is intentionally broad so the dev Test menu surfaces
// any element that does not render correctly.
const exhaustiveMarkdown = `# Heading level 1

## Heading level 2

### Heading level 3

#### Heading level 4

##### Heading level 5

###### Heading level 6

This paragraph mixes **bold text**, _italic text_, **_bold italic_**, ` + "`inline code`" + `, and ~~strikethrough~~ to check inline styling.

## Links and images

- [External link](https://example.com)
- [Reference-style link][ref]
- Autolink: <https://example.com>

[ref]: https://example.com "Reference title"

## Lists

Unordered:

- First item
- Second item
  - Nested item
    - Deeper nested item
- Third item

Ordered:

1. Step one
2. Step two
   1. Sub-step a
   2. Sub-step b
3. Step three

Task list:

- [x] Completed task
  - [x] Subtask A1
  - [x] Subtask A2
- [ ] Pending task
  - [x] Subtask B1
  - [ ] Subtask B2

## Blockquote

> A blockquote spanning a single line.
>
> > A nested blockquote with **bold** inside.


## Code


Inline ` + "`code`" + ` then a fenced block:


` + "```go" + `
package main

import "fmt"

func main() {
	fmt.Println("Hello, Markdown!")
	fmt.Println("This is a deliberately long code-block line that exceeds one hundred and twenty characters so the GUI renderer can test wrapping, clipping, or horizontal scrolling behavior.")
	veryLongIdentifierNameUsedToExerciseCodeBlockOverflowRendering := "a long string literal with enough content to push past the usual editor guide and expose layout issues inside fenced code blocks"
	fmt.Println(veryLongIdentifierNameUsedToExerciseCodeBlockOverflowRendering)
}
` + "```" + `


## Table

The "Rendered" column exercises inline formatting **inside** cells, and the
columns use left / center / right alignment.

| Style      | Syntax              | Rendered                       |
| :--------- | :-----------------: | -----------------------------: |
| Bold       | ` + "`**bold**`" + `          | **bold**                       |
| Italic     | ` + "`_italic_`" + `           | _italic_                       |
| Inline code | ` + "`print(\"hi\")`" + `      | ` + "`print(\"hi\")`" + `                 |
| Hyperlink  | ` + "`[link](url)`" + `        | [Example](https://example.com) |
| Strikethrough | ` + "`~~old~~`" + `         | ~~old~~                        |
| Long prose | A deliberately verbose cell that should wrap across several visual lines instead of forcing the table wider than the viewport. It includes enough ordinary words to reveal whether width calculation, soft wrapping, and row height expansion stay in sync. | The rendered side also contains a long sentence with **bold emphasis**, _italic emphasis_, and a [link](https://example.com) so wrapping can be checked when inline spans have different styles. |
| Tall content | First line<br>Second line with more text that should wrap when the table is narrow<br>Third line<br>Fourth line, still part of the same cell | This row simulates vertical spanning by making one cell much taller than its neighbors, which helps expose broken row height measurement and border drawing. |
| Narrow stress | supercalifragilisticexpialidocious-with-hyphenated-segments-and_a_long_identifier_like_this_one | A long unbroken token plus a second phrase after it should show where wrapping fails, clips, or overflows horizontally. |

## Horizontal rule

---

That concludes the markdown feature test.
`
