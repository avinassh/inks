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
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/junk"
)

var apContext = "https://www.w3.org/ns/activitystreams"
var apPublic = "https://www.w3.org/ns/activitystreams#Public"
var apTypes = []string{
	`application/activity+json`,
	`application/ld+json`,
}
var apBestType = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

var serverPubKey = "somekey"
var serverPrivateKey *rsa.PrivateKey

func isActivity(ct string) bool {
	ct = strings.ToLower(ct)
	for _, at := range apTypes {
		if strings.HasPrefix(ct, at) {
			return true
		}
	}
	return false
}

func apDeliver(tries int, rcpt string, msg []byte) error {
	if tries > 0 {
		time.Sleep(1 * time.Hour)
	}
	err := postMsg(rcpt, msg)
	if err != nil {
		log.Printf("error posting to %s: %s", rcpt, err)
		if tries != -1 && tries < 3 {
			go apDeliver(tries+1, rcpt, msg)
		}
	}
	return err
}

func oneLink(linkid int64) *Link {
	rows, err := stmtGetLink.Query(linkid)
	links, _ := readlinks(rows, err)
	if len(links) == 0 {
		return nil
	}
	return links[0]
}

func apHandle(w http.ResponseWriter, r *http.Request, linkid int64) {
	w.Header().Set("Cache-Control", "max-age=300")
	if r.URL.Path == "/" {
		apActor(w, r)
		return
	}
	link := oneLink(linkid)
	if link == nil {
		http.NotFound(w, r)
		return
	}

	jlink := apNote(link)
	jlink["@context"] = apContext

	w.Header().Set("Content-Type", apBestType)
	jlink.Write(w)
}

func apFinger(w http.ResponseWriter, r *http.Request) {
	j := junk.New()
	j["subject"] = fmt.Sprintf("acct:%s@%s", "inks", serverName)
	j["aliases"] = []string{serverURL}
	var links []junk.Junk
	l := junk.New()
	l["rel"] = "self"
	l["type"] = `application/activity+json`
	l["href"] = serverURL
	links = append(links, l)
	j["links"] = links

	w.Header().Set("Content-Type", "application/jrd+json")
	j.Write(w)
}

func apActor(w http.ResponseWriter, r *http.Request) {
	j := junk.New()
	j["@context"] = apContext
	j["id"] = serverURL
	j["type"] = "Application"
	j["inbox"] = serverURL + "/inbox"
	j["outbox"] = serverURL + "/outbox"
	j["followers"] = serverURL + "/followers"
	j["following"] = serverURL + "/following"
	j["name"] = serverName
	j["preferredUsername"] = "inks"
	j["summary"] = serverName
	j["url"] = serverURL
	a := junk.New()
	a["type"] = "Image"
	a["mediaType"] = "image/png"
	a["url"] = serverURL + "/icon.png"
	j["icon"] = a
	k := junk.New()
	k["id"] = serverURL + "#key"
	k["owner"] = serverURL
	k["publicKeyPem"] = serverPubKey
	j["publicKey"] = k

	w.Header().Set("Content-Type", apBestType)
	j.Write(w)
}

type Box struct {
	In     string
	Out    string
	Shared string
}

var boxcache = make(map[string]*Box)
var boxlock sync.Mutex

func getBoxes(actor string) (*Box, error) {
	boxlock.Lock()
	b, ok := boxcache[actor]
	boxlock.Unlock()
	if ok {
		return b, nil
	}
	j, err := junk.Get(actor, junk.GetArgs{Accept: apTypes[0], Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	b = new(Box)
	b.In, _ = j.GetString("inbox")
	b.Shared, _ = j.FindString([]string{"endpoints", "sharedInbox"})
	boxlock.Lock()
	boxcache[actor] = b
	boxlock.Unlock()
	return b, nil
}

func apAccept(req junk.Junk) {
	actor, _ := req.GetString("actor")

	j := junk.New()
	j["@context"] = apContext
	j["id"] = serverURL + "/accept/" + randomxid()
	j["type"] = "Accept"
	j["actor"] = serverURL
	j["to"] = actor
	j["published"] = time.Now().UTC().Format(time.RFC3339)
	j["object"] = req

	var buf bytes.Buffer
	j.Write(&buf)
	msg := buf.Bytes()

	box, err := getBoxes(actor)
	if err != nil {
		return
	}
	err = apDeliver(-1, box.In, msg)
	if err == nil {
		stmtSaveFollower.Exec(actor)
	}
}

func apInbox(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	payload := buf.Bytes()
	j, err := junk.Read(bytes.NewReader(payload))
	if err != nil {
		log.Printf("bad payload: %s", err)
		http.Error(w, "bad payload", http.StatusNotAcceptable)
	}
	what, _ := j.GetString("type")
	switch what {
	case "Create":
	case "Follow":
	case "Undo":
	default:
		return
	}
	keyname, err := httpsig.VerifyRequest(r, payload, httpsig.ActivityPubKeyGetter)
	who, _ := j.GetString("actor")
	if !strings.HasPrefix(keyname, who) {
		log.Printf("suspected forgery: %s vs %s", keyname, who)
		return
	}
	switch what {
	case "Create":
		fd, _ := os.OpenFile("savedinbox.json", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		j.Write(fd)
		io.WriteString(fd, "\n")
		fd.Close()
	case "Follow":
		obj, _ := j.GetString("object")
		if obj == serverURL {
			go apAccept(j)
		}
	case "Undo":
		obj, ok := j.GetMap("object")
		if ok {
			what, _ := obj.GetString("type")
			if what == "Follow" {
				stmtDeleteFollower.Exec(who)
			}
		}
	}
}

func apContent(link *Link) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<p><a href="%s">%s</a><p>`, html.EscapeString(link.URL), html.EscapeString(link.URL)))

	sb.WriteString(string(link.Summary))

	sb.WriteString("<p>")
	for _, tag := range link.Tags {
		sb.WriteString(fmt.Sprintf(`<a href="%s/tag/%s">#%s</a> `, serverURL, tag, tag))
	}
	return sb.String()
}

func apNote(link *Link) junk.Junk {
	j := junk.New()
	j["attributedTo"] = serverURL
	j["content"] = apContent(link)
	j["context"] = fmt.Sprintf("tag:%s:inks-%d", tagName, link.ID)
	j["conversation"] = j["context"]
	j["id"] = fmt.Sprintf("%s/l/%d", serverURL, link.ID)
	j["published"] = link.Posted.Format(time.RFC3339)
	j["summary"] = link.Title
	j["to"] = apPublic
	j["cc"] = serverURL + "/followers"
	j["type"] = "Note"
	j["url"] = j["id"]
	var tags []junk.Junk
	for _, tag := range link.Tags {
		t := junk.New()
		t["type"] = "Hashtag"
		t["name"] = "#" + tag
		t["url"] = serverURL + "/tag/" + tag
		tags = append(tags, t)
	}
	j["tag"] = tags

	return j
}

func apCreate(link *Link, update bool) junk.Junk {
	j := junk.New()
	j["actor"] = serverURL
	j["id"] = fmt.Sprintf("%s/l/%d/create", serverURL, link.ID)
	j["object"] = apNote(link)
	j["published"] = link.Posted.Format(time.RFC3339)
	j["to"] = apPublic
	j["cc"] = serverURL + "/followers"
	if update {
		j["type"] = "Update"
	} else {
		j["type"] = "Create"
	}
	return j
}

func apPublish(linkid int64, update bool) {
	if !update {
		// wait a minute for things to settle
		time.Sleep(1 * time.Minute)
	}
	link := oneLink(linkid)
	if link == nil {
		return
	}
	if update && link.Posted.After(time.Now().Add(-1*time.Minute)) {
		log.Printf("skipping update for new link")
		return
	}
	addrs := make(map[string]bool)
	rows, err := stmtGetFollowers.Query()
	if err != nil {
		log.Printf("error getting followers")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var actor string
		rows.Scan(&actor)
		box, _ := getBoxes(actor)
		if box != nil {
			if box.Shared != "" {
				addrs[box.Shared] = true
			} else {
				addrs[box.In] = true
			}
		}
	}
	j := apCreate(link, update)
	j["@context"] = apContext
	var buf bytes.Buffer
	j.Write(&buf)
	msg := buf.Bytes()
	for addr := range addrs {
		apDeliver(0, addr, msg)
	}
}

func apOutbox(w http.ResponseWriter, r *http.Request) {
	lastlink := 123456789012
	rows, err := stmtGetLinks.Query(lastlink)
	links, _ := readlinks(rows, err)

	var jlinks []junk.Junk
	for _, l := range links {
		j := apCreate(l, false)
		jlinks = append(jlinks, j)
	}

	j := junk.New()
	j["@context"] = apContext
	j["id"] = serverURL + "/outbox"
	j["type"] = "OrderedCollection"
	j["totalItems"] = len(jlinks)
	j["orderedItems"] = jlinks

	w.Header().Set("Content-Type", apBestType)
	j.Write(w)
}

func ap403(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "no", http.StatusForbidden)
}

func randomxid() string {
	letters := "BCDFGHJKLMNPQRSTVWXYZbcdfghjklmnpqrstvwxyz1234567891234567891234"
	var b [18]byte
	rand.Read(b[:])
	for i, c := range b {
		b[i] = letters[c&63]
	}
	s := string(b[:])
	return s
}

func postMsg(url string, msg []byte) error {
	client := http.DefaultClient
	req, err := http.NewRequest("POST", url, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", apBestType)
	httpsig.SignRequest(serverURL+"#key", serverPrivateKey, req, msg)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 201:
	case 202:
	default:
		return fmt.Errorf("http post status: %d", resp.StatusCode)
	}
	log.Printf("successful post: %s %d", url, resp.StatusCode)
	return nil
}
