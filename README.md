# go-fAST

`go-fAST` is a super-fast Golang library designed for parsing, transforming, and generating JavaScript Abstract Syntax Trees (ASTs). This library provides a simple and efficient way to work with JavaScript ASTs in Go, enabling developers to perform a variety of tasks including code analysis and (de)obfuscation.

[![Discord Banner2](https://discord.com/api/guilds/1288989871498858496/widget.png?style=banner2)](https://discord.gg/wdJ4VcdrwK)

## Features

- **Parsing**: Convert JavaScript code into its AST representation.
- **Transforming**: Apply transformations to JavaScript ASTs to modify code structures.
- **Generating**: Generate JavaScript code from ASTs.
- **Comments**: Optional comment capture and reprint (off by default).

## Installation

To use `go-fAST` in your Go project, you need to install it using `go get`:

```sh
go get github.com/t14raptor/go-fast
```

## Docs

[Documentation](https://gofast.disasm.dev)

## Comments

Comments are **off by default**. They are not AST nodes and the visitor does not walk them. The parser stores them in a span table on `*ast.Program` (`Comments` + `Source`). This is the same model as [SWC](https://swc.rs/) and [Oxc](https://oxc.rs/).

```go
prog, err := parser.ParseWithOptions(src, parser.Options{Comments: true})
out := generator.GenerateWithOptions(prog, generator.Options{Comments: true})
```

`parser.Parse` and `generator.Generate` skip comments.

Each `ast.Comment` holds spans and an attach point. Text is read from `prog.Source` via `Comment.Text` / `Comment.Body`. Use `ast.Leading` and `ast.Trailing` to filter by token start.

A rewrite that changes a node’s `Idx` must retarget the table:

```go
ast.Move(prog.Comments, old.Idx0(), new.Idx0())
```

`ext.RemoveHelper` prunes the table on delete. Legal comments (`@license`, `@preserve`, `/*!`, `//!`) are kept and retargeted. Clone-in-place needs no call. A clone inserted elsewhere keeps comments on the original span.

Pretty generate is a decompiler, not a byte copy. Comment text is kept; column and nearby tokens may shift. A trailing `//` always gets a newline so it cannot swallow the next token.

Minify with `Comments: true` drops normal `//` and JSDoc. It keeps legal comments, `@__PURE__`, `@__NO_SIDE_EFFECTS__`, and V8 dump tags. A line `// @__PURE__` is rewritten to `/* @__PURE__ */`.

Not included: comments on individual nodes, auto-`Move`, source-identical print, HTML / TypeScript / JSX comment forms.

## Credits

We'd like to extend our heartfelt thanks to the following individuals and projects for their invaluable contributions:

- **[@JustTalDevelops](https://github.com/JustTalDevelops)**: For providing exceptional assistance with code generation edge cases and pretty printing.
- **[@steakenthusiast](https://github.com/steakenthusiast)**: For the creative and fitting name for the project.
- **[goja](https://github.com/dop251/goja)**: For their parsing code that was instrumental in our implementation.
- **[swc](https://swc.rs/)**: For their robust and efficient visiting API.

Your support and contributions have greatly enhanced the development of this project. Thank you!
