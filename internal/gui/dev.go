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
- [ ] Pending task

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

## Horizontal rule

---

That concludes the markdown feature test.
`
