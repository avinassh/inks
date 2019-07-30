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

	out := `<blockquote>&gt; we start with a quote</blockquote>
<p>A comment. I liked it.
<p><blockquote>&gt; feature one<br>
&gt; feature two<br>
&gt; feature three</blockquote>
<p>nice!`
	rv := string(htmlify(in))
	if rv != out {
		t.Errorf("failure.\nresult: %s\nexpected: %s\n", rv, out)
	}

in = `> one quote

> two quote`
	out = `<blockquote>&gt; one quote</blockquote>
<p><blockquote>&gt; two quote</blockquote>`

	rv = string(htmlify(in))
	if rv != out {
		t.Errorf("failure.\nresult: %s\nexpected: %s\n", rv, out)
	}
}
