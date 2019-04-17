//
// Copyright (c) 2019 Ted Unangst <tedu@tedunangst.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import (
	"html/template"
	"io"
	"log"
)

type Template struct {
	names     []string
	templates *template.Template
	reload    bool
}

func (t *Template) ExecuteTemplate(w io.Writer, name string, data interface{}) error {
	if t.reload {
		templates, err := template.ParseFiles(t.names...)
		if err != nil {
			return err
		}
		return templates.ExecuteTemplate(w, name, data)
	}
	return t.templates.ExecuteTemplate(w, name, data)
}

func ParseTemplates(reload bool, filenames ...string) *Template {
	t := new(Template)
	t.names = filenames
	t.reload = reload
	templates, err := template.ParseFiles(t.names...)
	if err != nil {
		log.Panic(err)
	}
	if !reload {
		t.templates = templates
	}
	return t
}
