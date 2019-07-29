package main

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
)

func htmlify(s string) template.HTML {
	s = strings.Replace(s, "\r", "", -1)
	s = prettyquotes(s)
	s = html.EscapeString(s)

	linkfn := func(url string) string {
		addparen := false
		adddot := false
		if strings.HasSuffix(url, ")") && strings.IndexByte(url, '(') == -1 {
			url = url[:len(url)-1]
			addparen = true
		}
		if strings.HasSuffix(url, ".") {
			url = url[:len(url)-1]
			adddot = true
		}
		url = fmt.Sprintf(`<a href="%s">%s</a>`, url, url)
		if adddot {
			url += "."
		}
		if addparen {
			url += ")"
		}
		return url
	}
	re_link := regexp.MustCompile(`https?://[^\s"]+[\w/)]`)
	s = re_link.ReplaceAllStringFunc(s, linkfn)

	re_i := regexp.MustCompile("&gt; (.*)\n")
	s = re_i.ReplaceAllString(s, "<blockquote>&gt; $1</blockquote>\n")
	s = strings.ReplaceAll(s, "</blockquote>\n<blockquote>", "\n")
	renl := regexp.MustCompile("\n+")
	nlrepl := func(s string) string {
		if len(s) > 1 {
			return "\n<p>"
		}
		return "<br>\n"
	}
	s = renl.ReplaceAllStringFunc(s, nlrepl)

	return template.HTML(s)
}

func prettyquotes(s string) string {
	lq := "\u201c"
	rq := "\u201d"
	ls := "\u2018"
	rs := "\u2019"
	ap := rs
	re_lq := regexp.MustCompile(`"[^.\s]`)
	lq_fn := func(s string) string {
		return lq + s[1:]
	}
	s = re_lq.ReplaceAllStringFunc(s, lq_fn)
	s = strings.Replace(s, `"`, rq, -1)
	re_ap := regexp.MustCompile(`\w'`)
	ap_fn := func(s string) string {
		return s[0:len(s)-1] + ap
	}
	s = re_ap.ReplaceAllStringFunc(s, ap_fn)
	re_ls := regexp.MustCompile(`'\w`)
	ls_fn := func(s string) string {
		return ls + s[1:]
	}
	s = re_ls.ReplaceAllStringFunc(s, ls_fn)
	s = strings.Replace(s, `'`, rs, -1)

	return s
}
