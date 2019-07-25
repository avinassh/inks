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
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/login"
	"humungus.tedunangst.com/r/webs/rss"
	"humungus.tedunangst.com/r/webs/templates"
)

var readviews *templates.Template

var serverName = "localhost"
var serverURL = "https://localhost"
var tagName = "inks,2019"

func getInfo(r *http.Request) map[string]interface{} {
	templinfo := make(map[string]interface{})
	templinfo["StyleParam"] = getstyleparam("views/style.css")
	templinfo["UserInfo"] = login.GetUserInfo(r)
	templinfo["LogoutCSRF"] = login.GetCSRF("logout", r)
	return templinfo
}

type Link struct {
	ID           int64
	URL          string
	Posted       time.Time
	Source       string
	Site         string
	Title        string
	Tags         []string
	PlainSummary string
	Summary      template.HTML
	Edit         string
}

func taglinks(links []*Link) {
	db := opendatabase()
	var ids []string
	lmap := make(map[int64]*Link)
	for _, l := range links {
		ids = append(ids, fmt.Sprintf("%d", l.ID))
		lmap[l.ID] = l
	}
	q := fmt.Sprintf("select linkid, tag from tags where linkid in (%s)", strings.Join(ids, ","))
	rows, err := db.Query(q)
	if err != nil {
		log.Printf("can't load tags: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var lid int64
		var t string
		err = rows.Scan(&lid, &t)
		if err != nil {
			log.Printf("can't scan tag: %s", err)
			continue
		}
		l := lmap[lid]
		l.Tags = append(l.Tags, t)
	}
	for _, l := range links {
		sort.Strings(l.Tags)
	}
}

func searchlinks(search string, lastlink int64) ([]*Link, int64) {
	if !regexp.MustCompile(`^["[:alnum:]_ -]*$`).MatchString(search) {
		search = ""
	}
	quotes := 0
	for _, c := range search {
		if c == '"' {
			quotes++
		}
	}
	if quotes%2 == 1 {
		search = search + `"`
	}
	log.Printf("searching for '%s'", search)
	rows, err := stmtSearchLinks.Query(search, lastlink)
	return readlinks(rows, err)
}

func readlinks(rows *sql.Rows, err error) ([]*Link, int64) {
	if err != nil {
		log.Printf("error getting links: %s", err)
		return nil, 0
	}
	var lastlink int64
	var links []*Link
	for rows.Next() {
		var link Link
		var dt string
		err = rows.Scan(&link.ID, &link.URL, &dt, &link.Source, &link.Site, &link.Title, &link.PlainSummary)
		if err != nil {
			log.Printf("error scanning link: %s", err)
			continue
		}
		link.Posted, _ = time.Parse(dbtimeformat, dt)
		link.Summary = htmlify(link.PlainSummary)
		links = append(links, &link)
		lastlink = link.ID
	}
	rows.Close()
	taglinks(links)
	return links, lastlink
}

func showlinks(w http.ResponseWriter, r *http.Request) {
	lastlink, _ := strconv.ParseInt(mux.Vars(r)["lastlink"], 10, 0)
	linkid, _ := strconv.ParseInt(mux.Vars(r)["linkid"], 10, 0)
	sourcename := mux.Vars(r)["sourcename"]
	sitename := mux.Vars(r)["sitename"]
	tagname := mux.Vars(r)["tagname"]
	search := r.FormValue("q")

	if isActivity(r.Header.Get("Accept")) {
		apHandle(w, r, linkid)
		return
	}

	var links []*Link
	if linkid > 0 {
		rows, err := stmtGetLink.Query(linkid)
		links, _ = readlinks(rows, err)
	} else if r.URL.Path == "/random" {
		rows, err := stmtRandomLinks.Query()
		links, _ = readlinks(rows, err)
	} else {
		if lastlink == 0 {
			lastlink = 123456789012
		}
		if search != "" {
			links, lastlink = searchlinks(search, lastlink)
		} else if tagname != "" {
			rows, err := stmtTagLinks.Query(tagname, lastlink)
			links, lastlink = readlinks(rows, err)
		} else if sourcename != "" {
			rows, err := stmtSourceLinks.Query(sourcename, lastlink)
			links, lastlink = readlinks(rows, err)
		} else if sitename != "" {
			rows, err := stmtSiteLinks.Query(sitename, lastlink)
			links, lastlink = readlinks(rows, err)
		} else {
			rows, err := stmtGetLinks.Query(lastlink)
			links, lastlink = readlinks(rows, err)
		}
	}

	if login.GetUserInfo(r) == nil {
		w.Header().Set("Cache-Control", "max-age=300")
	}

	templinfo := getInfo(r)
	templinfo["Links"] = links
	templinfo["LastLink"] = lastlink
	templinfo["SaveCSRF"] = login.GetCSRF("savelink", r)
	err := readviews.Execute(w, "inks.html", templinfo)
	if err != nil {
		log.Printf("error templating inks: %s", err)
	}
}

var re_sitename = regexp.MustCompile("//([^/]+)/")

func savelink(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	summary := r.FormValue("summary")
	tags := r.FormValue("tags")
	source := r.FormValue("source")
	linkid, _ := strconv.ParseInt(r.FormValue("linkid"), 10, 0)

	title = strings.TrimSpace(title)
	if strings.ToUpper(title) == title && strings.IndexByte(title, ' ') != -1 {
		title = strings.Title(strings.ToLower(title))
	}
	summary = strings.TrimSpace(summary)
	tags = strings.TrimSpace(tags)
	source = strings.TrimSpace(source)
	site := re_sitename.FindString(url)
	if site != "" {
		site = site[2 : len(site)-1]
	}
	dt := time.Now().UTC().Format(dbtimeformat)

	log.Printf("save link: %s", url)

	res, err := stmtSaveSummary.Exec(title, summary, url)
	if err != nil {
		log.Printf("error saving summary: %s", err)
		return
	}
	textid, _ := res.LastInsertId()
	if linkid > 0 {
		stmtDeleteTags.Exec(linkid)
		_, err = stmtUpdateLink.Exec(textid, url, source, site, linkid)
	} else {
		res, err = stmtSaveLink.Exec(textid, url, dt, source, site)
		linkid, _ = res.LastInsertId()
		go apPublish(linkid)
	}
	if err != nil {
		log.Printf("error saving link: %s", err)
		return
	}
	for _, t := range strings.Split(tags, " ") {
		stmtSaveTag.Exec(linkid, t)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func showrss(w http.ResponseWriter, r *http.Request) {
	log.Printf("view rss")
	home := fmt.Sprintf("https://%s/", serverName)
	feed := rss.Feed{
		Title:       "inks",
		Link:        home,
		Description: "inks rss",
		Image: &rss.Image{
			URL:   home + "icon.png",
			Title: "inks rss",
			Link:  home,
		},
	}
	var modtime time.Time
	rows, err := stmtGetLinks.Query(123456789012)
	links, _ := readlinks(rows, err)
	for _, link := range links {
		tag := fmt.Sprintf("tag:%s:inks-%d", tagName, link.ID)
		summary := string(link.Summary)
		if link.Source != "" {
			summary += "\n<p>source: " + link.Source
		}
		feed.Items = append(feed.Items, &rss.Item{
			Title:       link.Title,
			Description: rss.CData{string(link.Summary)},
			Category:    link.Tags,
			Link:        link.URL,
			PubDate:     link.Posted.Format(time.RFC1123),
			Guid:        &rss.Guid{Value: tag},
		})
		if link.Posted.After(modtime) {
			modtime = link.Posted
		}
	}
	w.Header().Set("Cache-Control", "max-age=300")
	w.Header().Set("Last-Modified", modtime.Format(http.TimeFormat))

	err = feed.Write(w)
	if err != nil {
		log.Printf("error writing rss: %s", err)
	}
}

func servecss(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=7776000")
	http.ServeFile(w, r, "views"+r.URL.Path)
}
func servehtml(w http.ResponseWriter, r *http.Request) {
	templinfo := getInfo(r)
	err := readviews.Execute(w, r.URL.Path[1:]+".html", templinfo)
	if err != nil {
		log.Print(err)
	}
}
func serveform(w http.ResponseWriter, r *http.Request) {
	linkid, _ := strconv.ParseInt(mux.Vars(r)["linkid"], 10, 0)
	link := new(Link)
	if linkid > 0 {
		rows, err := stmtGetLink.Query(linkid)
		links, _ := readlinks(rows, err)
		link = links[0]
	}
	templinfo := getInfo(r)
	templinfo["SaveCSRF"] = login.GetCSRF("savelink", r)
	templinfo["Link"] = link
	err := readviews.Execute(w, "addlink.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

var stmtGetLink, stmtGetLinks, stmtSearchLinks, stmtSaveSummary, stmtSaveLink *sql.Stmt
var stmtTagLinks, stmtSiteLinks, stmtSourceLinks, stmtDeleteTags, stmtUpdateLink, stmtSaveTag *sql.Stmt
var stmtRandomLinks *sql.Stmt
var stmtGetFollowers, stmtSaveFollower, stmtDeleteFollower *sql.Stmt

func preparetodie(db *sql.DB, s string) *sql.Stmt {
	stmt, err := db.Prepare(s)
	if err != nil {
		log.Fatalf("error %s: %s", err, s)
	}
	return stmt
}

func prepareStatements(db *sql.DB) {
	stmtGetLink = preparetodie(db, "select linkid, url, dt, source, site, title, summary from links join linktext on links.textid = linktext.docid where linkid = ?")
	stmtGetLinks = preparetodie(db, "select linkid, url, dt, source, site, title, summary from links join linktext on links.textid = linktext.docid where linkid < ? order by linkid desc limit 20")
	stmtSearchLinks = preparetodie(db, "select linkid, url, dt, source, site, title, summary from links join linktext on links.textid = linktext.docid where linktext match ? and linkid < ? order by linkid desc limit 20")
	stmtTagLinks = preparetodie(db, "select linkid, url, dt, source, site, title, summary from links join linktext on links.textid = linktext.docid where linkid in (select linkid from tags where tag = ?) and linkid < ? order by linkid desc limit 20")
	stmtSourceLinks = preparetodie(db, "select linkid, url, dt, source, site, title, summary from links join linktext on links.textid = linktext.docid where source = ? and linkid < ? order by linkid desc limit 20")
	stmtSiteLinks = preparetodie(db, "select linkid, url, dt, source, site, title, summary from links join linktext on links.textid = linktext.docid where site = ? and linkid < ? order by linkid desc limit 20")
	stmtRandomLinks = preparetodie(db, "select linkid, url, dt, source, site, title, summary from links join linktext on links.textid = linktext.docid order by random() limit 20")
	stmtSaveSummary = preparetodie(db, "insert into linktext (title, summary, remnants) values (?, ?, ?)")
	stmtSaveLink = preparetodie(db, "insert into links (textid, url, dt, source, site) values (?, ?, ?, ?, ?)")
	stmtUpdateLink = preparetodie(db, "update links set textid = ?, url = ?, source = ?, site = ? where linkid = ?")
	stmtDeleteTags = preparetodie(db, "delete from tags where linkid = ?")
	stmtSaveTag = preparetodie(db, "insert into tags (linkid, tag) values (?, ?)")
	stmtGetFollowers = preparetodie(db, "select url from followers")
	stmtSaveFollower = preparetodie(db, "insert into followers (url) values (?)")
	stmtDeleteFollower = preparetodie(db, "delete from followers where url = ?")
}

func serve() {
	db := opendatabase()
	prepareStatements(db)
	login.Init(db)

	listener, err := openListener()
	if err != nil {
		log.Fatal(err)
	}

	getconfig("servername", &serverName)
	serverURL = "https://" + serverName
	getconfig("pubkey", &serverPubKey)
	var seckey string
	getconfig("seckey", &seckey)
	serverPrivateKey, _, _ = httpsig.DecodeKey(seckey)

	tagName = fmt.Sprintf("%s,%d", serverName, 2019)

	debug := false
	getconfig("debug", &debug)

	readviews = templates.Load(debug,
		"views/header.html",
		"views/inks.html",
		"views/addlink.html",
		"views/login.html",
	)
	if !debug {
		s := "views/style.css"
		savedstyleparams[s] = getstyleparam(s)

	}

	mux := mux.NewRouter()
	mux.Use(login.Checker)

	getters := mux.Methods("GET").Subrouter()
	getters.HandleFunc("/", showlinks)
	getters.HandleFunc("/search", showlinks)
	getters.HandleFunc("/before/{lastlink:[0-9]+}", showlinks)
	getters.HandleFunc("/l/{linkid:[0-9]+}", showlinks)
	getters.Handle("/edit/{linkid:[0-9]+}", login.Required(http.HandlerFunc(serveform)))
	getters.HandleFunc("/site/{sitename:[[:alnum:].-]+}", showlinks)
	getters.HandleFunc("/source/{sourcename:[[:alnum:].-]+}", showlinks)
	getters.HandleFunc("/tag/{tagname:[[:alnum:].-]+}", showlinks)
	getters.HandleFunc("/random", showlinks)
	getters.HandleFunc("/rss", showrss)
	getters.HandleFunc("/style.css", servecss)
	getters.HandleFunc("/login", servehtml)
	getters.Handle("/addlink", login.Required(http.HandlerFunc(serveform)))
	getters.HandleFunc("/logout", login.LogoutFunc)

	posters := mux.Methods("POST").Subrouter()
	posters.Handle("/savelink", login.CSRFWrap("savelink", http.HandlerFunc(savelink)))
	posters.HandleFunc("/dologin", login.LoginFunc)

	getters.HandleFunc("/.well-known/webfinger", apFinger)
	getters.HandleFunc("/outbox", apOutbox)
	getters.HandleFunc("/followers", ap403)
	getters.HandleFunc("/following", ap403)
	posters.HandleFunc("/inbox", apInbox)

	err = http.Serve(listener, mux)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "init":
		initdb()
	case "run":
		serve()
	default:
		log.Fatal("unknown command")
	}
}
