package main

import "testing"

func TestHtml(t *testing.T) {
	in :=
		`> we start with a quote

A comment. I liked it.

> feature one
> feature two
> feature three

nice!
`

	out := `<blockquote>we start with a quote</blockquote>
<p>A comment. I liked it.
<p><blockquote>feature one<br>
feature two<br>
feature three</blockquote>
<p>nice!`
	rv := string(htmlify(in))
	if rv != out {
		t.Errorf("failure.\nresult: %s\nexpected: %s\n", rv, out)
	}

	in = `> one quote

> two quote`
	out = `<blockquote>one quote</blockquote>
<p><blockquote>two quote</blockquote>`

	rv = string(htmlify(in))
	if rv != out {
		t.Errorf("failure.\nresult: %s\nexpected: %s\n", rv, out)
	}
}
