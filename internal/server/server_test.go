package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aloglu/bookshelf/internal/auth"
	"github.com/aloglu/bookshelf/internal/backup"
	"github.com/aloglu/bookshelf/internal/lookup"
	"github.com/aloglu/bookshelf/internal/store"
)

type env struct {
	t       *testing.T
	srv     *httptest.Server
	owner   *http.Client // signed in
	visitor *http.Client
	st      *store.Store
	dir     string
}

func setup(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a, err := auth.New(st, "admin", auth.Plain("secret"), "")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{Store: st, Auth: a, Lookup: lookup.New("", "test"), Backups: backup.New(dir, st, "test", log), DataDir: dir, Version: "test", Log: log,
		Web: fstest.MapFS{"index.html": {Data: []byte(testPage)}}}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	e := &env{t: t, srv: srv, owner: &http.Client{Jar: jar}, visitor: &http.Client{}, st: st, dir: dir}
	if code, _ := e.do(e.owner, "POST", "/api/session", map[string]string{"user": "admin", "password": "secret"}); code != 200 {
		t.Fatalf("sign in: %d", code)
	}
	return e
}

// testPage has the built page's head, as the server rewrites it.
const testPage = `<!doctype html>
<head>
  <title>Bookshelf</title>
  <link rel="icon" href="/favicon.svg" type="image/svg+xml">
  <link rel="icon" href="/favicon-32.png" sizes="32x32" type="image/png">
  <link rel="apple-touch-icon" href="/apple-touch-icon.png">
  <link rel="manifest" href="/manifest.json">
</head>`

// do sends JSON (or raw bytes) the way the site's own page does, and decodes the answer.
func (e *env) do(c *http.Client, method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Origin", e.srv.URL)
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (e *env) addBook(title, isbn string, hidden bool) string {
	e.t.Helper()
	code, b := e.do(e.owner, "POST", "/api/books", map[string]any{"title": title, "author": "A. Writer", "identifier": isbn, "pages": 300, "hidden": hidden})
	if code != 201 {
		e.t.Fatalf("add %s: %d %v", title, code, b)
	}
	return b["id"].(string)
}

func titles(lib map[string]any) []string {
	var out []string
	for _, b := range lib["books"].([]any) {
		out = append(out, b.(map[string]any)["title"].(string))
	}
	return out
}

func png1(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 200, 300))
	for y := range 300 {
		for x := range 200 {
			img.Set(x, y, color.RGBA{30, 60, 120, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestVisitorsNeverSeeHiddenBooks(t *testing.T) {
	e := setup(t)
	e.addBook("Shown", "9780679723424", false)
	hid := e.addBook("Secret", "", true)
	e.do(e.owner, "PUT", "/api/books/"+hid+"/cover", png1(t))

	_, mine := e.do(e.owner, "GET", "/api/library", nil)
	if got := strings.Join(titles(mine), ","); got != "Shown,Secret" {
		t.Fatalf("owner sees %s", got)
	}
	_, theirs := e.do(e.visitor, "GET", "/api/library", nil)
	if got := strings.Join(titles(theirs), ","); got != "Shown" {
		t.Fatalf("visitor sees %s", got)
	}
	if _, ok := theirs["settings"].(map[string]any)["private"]; ok {
		t.Fatal("visitors received owner-only settings")
	}
	// The hidden book's cover isn't served to visitors either.
	var cover string
	for _, b := range mine["books"].([]any) {
		if bm := b.(map[string]any); bm["id"] == hid {
			cover = bm["cover"].(map[string]any)["url"].(string)
		}
	}
	if resp, _ := e.owner.Get(e.srv.URL + cover); resp.StatusCode != 200 {
		t.Fatalf("owner cover: %d", resp.StatusCode)
	}
	if resp, _ := e.visitor.Get(e.srv.URL + cover); resp.StatusCode != 404 {
		t.Fatalf("visitor fetched a hidden book's cover: %d", resp.StatusCode)
	}
}

func TestPrivateLibrary(t *testing.T) {
	e := setup(t)
	e.addBook("Anything", "", false)
	if code, _ := e.do(e.owner, "PATCH", "/api/settings", map[string]any{"private": true}); code != 200 {
		t.Fatal("could not make it private")
	}
	code, body := e.do(e.visitor, "GET", "/api/library", nil)
	if code != 403 || body["private"] != true || body["books"] != nil {
		t.Fatalf("private library to a visitor: %d %v", code, body)
	}
	if code, _ := e.do(e.owner, "GET", "/api/library", nil); code != 200 {
		t.Fatal("the owner was refused their own private library")
	}
}

func TestOwnerOnly(t *testing.T) {
	e := setup(t)
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/books"}, {"PATCH", "/api/settings"}, {"GET", "/api/readings"}, {"GET", "/api/lookup?id=9780679723424"}, {"GET", "/api/updates"}, {"DELETE", "/api/books/x"},
	} {
		if code, _ := e.do(e.visitor, c.method, c.path, map[string]any{}); code != 401 {
			t.Errorf("%s %s by a visitor: %d", c.method, c.path, code)
		}
	}
}

func TestChangesFromOtherSitesAreRefused(t *testing.T) {
	e := setup(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/books", strings.NewReader(`{"title":"Planted"}`))
	req.Header.Set("Origin", "https://evil.example")
	resp, _ := e.owner.Do(req)
	if resp.StatusCode != 403 {
		t.Fatalf("cross-site change: %d", resp.StatusCode)
	}
	if n := len(e.st.Snapshot().Books); n != 0 {
		t.Fatal("the cross-site book was added")
	}
	try := func(site, origin string) int {
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/books", strings.NewReader(`{"title":"Planted"}`))
		req.Header.Set("Sec-Fetch-Site", site)
		req.Header.Set("Origin", origin)
		resp, err := e.owner.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	// A modern browser says where the request comes from, whatever its Origin claims.
	for _, site := range []string{"cross-site", "same-site"} {
		if code := try(site, e.srv.URL); code != 403 {
			t.Fatalf("Sec-Fetch-Site %s: %d", site, code)
		}
	}
	// Behind a proxy that rewrites Host, the browser still says same-origin.
	if code := try("same-origin", "https://books.example.com"); code != 201 {
		t.Fatalf("same-origin behind a proxy: %d", code)
	}
}

func TestEveryAnswerCarriesTheSafetyHeaders(t *testing.T) {
	e := setup(t)
	for _, path := range []string{"/", "/api/library", "/api/nothing"} {
		resp, err := e.visitor.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		h := resp.Header
		if h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "script-src 'self'") || h.Get("X-Frame-Options") != "SAMEORIGIN" {
			t.Fatalf("%s: %v", path, h)
		}
	}
}

func TestSignInMistakes(t *testing.T) {
	e := setup(t)
	c := &http.Client{}
	if code, _ := e.do(c, "POST", "/api/session", map[string]string{"user": "admin", "password": "wrong"}); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	for range 5 {
		e.do(c, "POST", "/api/session", map[string]string{"user": "admin", "password": "wrong"})
	}
	code, body := e.do(c, "POST", "/api/session", map[string]string{"user": "admin", "password": "secret"})
	if code != 429 || !strings.Contains(body["error"].(string), "Try again in") {
		t.Fatalf("lockout: %d %v", code, body)
	}
}

func TestBooksAndISBNs(t *testing.T) {
	e := setup(t)
	code, b := e.do(e.owner, "POST", "/api/books", map[string]any{"title": "Pale Fire", "identifier": "ISBN-10: 0-679-72342-0", "binding": "Mass market paperback"})
	if code != 201 || b["isbn"] != "9780679723424" || b["identifier"] != "978-0-679-72342-4" || b["binding"] != "paperback" || b["address"] != "9780679723424" {
		t.Fatalf("add: %d %v", code, b)
	}
	if code, body := e.do(e.owner, "POST", "/api/books", map[string]any{"title": "Again", "identifier": "9780679723424"}); code != 409 {
		t.Fatalf("duplicate: %d %v", code, body)
	}
	if code, _ := e.do(e.owner, "POST", "/api/books", map[string]any{"title": "Bad", "identifier": "9780679723425"}); code != 422 {
		t.Fatalf("bad checksum: %d", code)
	}
	e.do(e.owner, "PATCH", "/api/settings", map[string]any{"permalink": "title"})
	_, lib := e.do(e.owner, "GET", "/api/library", nil)
	if a := lib["books"].([]any)[0].(map[string]any)["address"]; a != "pale-fire" {
		t.Fatalf("address by title: %v", a)
	}
}

func TestPeriodicalIssues(t *testing.T) {
	e := setup(t)
	code, b := e.do(e.owner, "POST", "/api/books", map[string]any{"title": "Cogito 70", "identifier": "ISSN 1300-2880", "issue": " 70–71 "})
	if code != 201 || b["issn"] != "13002880" || b["issue"] != "70–71" || b["identifier"] != "ISSN 1300-2880" || b["isbn"] != "" || b["address"] != "cogito-70" {
		t.Fatalf("add: %d %v", code, b)
	}
	if code, body := e.do(e.owner, "POST", "/api/books", map[string]any{"title": "Again", "identifier": "13002880", "issue": "70–71"}); code != 409 {
		t.Fatalf("same issue: %d %v", code, body)
	}
	if code, body := e.do(e.owner, "POST", "/api/books", map[string]any{"title": "Cogito 76", "identifier": "1300-2880", "issue": "76"}); code != 201 {
		t.Fatalf("another issue: %d %v", code, body)
	}
	code, d := e.do(e.owner, "GET", "/api/lookup?id=977-2146023007", nil)
	if code != 200 || d["periodical"] != true || d["issn"] != "2146023X" || d["identifier"] != "ISSN 2146-023X" {
		t.Fatalf("lookup of a 977 barcode: %d %v", code, d)
	}
}

func TestTheOwnersOwnIcon(t *testing.T) {
	e := setup(t)
	get := func(path string) (int, string) {
		resp, err := e.visitor.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	e.do(e.owner, "PATCH", "/api/settings", map[string]any{"name": "Our <books>"})
	if _, page := get("/"); !strings.Contains(page, "<title>Our &lt;books&gt;</title>") || !strings.Contains(page, `href="/favicon.svg"`) {
		t.Fatalf("the page before an icon: %s", page)
	}
	code, st := e.do(e.owner, "PUT", "/api/icon", png1(t))
	name, _ := st["icon"].(string)
	if code != 200 || len(name) != 16 {
		t.Fatalf("upload: %d %v", code, st)
	}
	_, page := get("/9780679723424") // every address of the app is written the same way
	if strings.Contains(page, "favicon.svg") || !strings.Contains(page, `href="/icon/`+name+`-32.png"`) || !strings.Contains(page, `href="/icon/`+name+`-180.png"`) {
		t.Fatalf("the page with an icon: %s", page)
	}
	for _, size := range []string{"32", "180", "192", "512"} {
		if code, _ := get("/icon/" + name + "-" + size + ".png"); code != 200 {
			t.Fatalf("icon %s: %d", size, code)
		}
	}
	if code, _ := get("/icon/" + name + "-64.png"); code != 404 {
		t.Fatal("an icon size that doesn't exist was served")
	}
	_, man := get("/manifest.json")
	if !strings.Contains(man, `"name":"Our \u003cbooks\u003e"`) || !strings.Contains(man, "/icon/"+name+"-512.png") {
		t.Fatalf("manifest: %s", man)
	}
	// Back to Bookshelf's own: the files go, and the page links Bookshelf's icon again.
	if code, st := e.do(e.owner, "DELETE", "/api/icon", nil); code != 200 || st["icon"] != "" {
		t.Fatalf("back: %d %v", code, st)
	}
	if code, _ := get("/icon/" + name + "-32.png"); code != 404 {
		t.Fatal("the old icon is still served")
	}
	if entries, _ := os.ReadDir(filepath.Join(e.dir, "icon")); len(entries) != 0 {
		t.Fatalf("the old icon's files are left: %v", entries)
	}
	if _, page := get("/"); !strings.Contains(page, `href="/favicon.svg"`) {
		t.Fatal("the page doesn't link Bookshelf's icon again")
	}
	if code, _ := e.do(e.visitor, "PUT", "/api/icon", png1(t)); code != 401 {
		t.Fatal("a visitor set the icon")
	}
}

func TestCoverUploadSetsTheSpine(t *testing.T) {
	e := setup(t)
	id := e.addBook("Blue", "", false)
	code, b := e.do(e.owner, "PUT", "/api/books/"+id+"/cover", png1(t))
	if code != 200 || b["spine"].(map[string]any)["color"] != "#1e3c78" {
		t.Fatalf("cover: %d %v", code, b)
	}
	if code, _ := e.do(e.owner, "PUT", "/api/books/"+id+"/cover", []byte("nope")); code != 422 {
		t.Fatalf("not an image: %d", code)
	}
	code, b = e.do(e.owner, "PUT", "/api/books/"+id+"/spine", map[string]any{"color": "#AA0000"})
	if code != 200 || b["spine"].(map[string]any)["color"] != "#aa0000" || b["spine"].(map[string]any)["own"] != true {
		t.Fatalf("own spine: %d %v", code, b)
	}
}

func TestReadingsThroughTheAPI(t *testing.T) {
	e := setup(t)
	id := e.addBook("Austerlitz", "", false)
	code, rd := e.do(e.owner, "POST", "/api/readings", map[string]any{"bookId": id})
	if code != 201 {
		t.Fatalf("start: %d %v", code, rd)
	}
	rid := rd["id"].(string)
	if code, _ := e.do(e.owner, "PATCH", "/api/readings/"+rid, map[string]any{"page": 112}); code != 200 {
		t.Fatal("page")
	}
	_, lib := e.do(e.visitor, "GET", "/api/library", nil)
	if r := lib["reading"].([]any)[0].(map[string]any); r["page"] != 112.0 || r["bookId"] != id {
		t.Fatalf("visitors see the current reading: %v", r)
	}
	if code, _ := e.do(e.owner, "PATCH", "/api/readings/"+rid, map[string]any{"end": "finished"}); code != 200 {
		t.Fatal("finish")
	}
	if code, _ := e.do(e.owner, "PUT", "/api/readings/"+rid+"/note", map[string]any{"text": "Haunting.", "public": false}); code != 200 {
		t.Fatal("note")
	}
	if code, _ := e.do(e.owner, "POST", "/api/readings", map[string]any{"bookId": id, "read": map[string]any{"precision": "year", "date": "2019"}}); code != 201 {
		t.Fatal("mark read")
	}
	_, mine := e.do(e.owner, "GET", "/api/library", nil)
	if n := mine["books"].([]any)[0].(map[string]any)["readCount"]; n != 2.0 {
		t.Fatalf("read count %v", n)
	}
	_, theirs := e.do(e.visitor, "GET", "/api/library", nil)
	if _, ok := theirs["books"].([]any)[0].(map[string]any)["readCount"]; ok || len(theirs["reading"].([]any)) != 0 {
		t.Fatalf("visitors see the reading history: %v", theirs["books"])
	}
	if _, ok := theirs["books"].([]any)[0].(map[string]any)["notes"]; ok {
		t.Fatal("visitors see a private note")
	}

	// A public note is shown to visitors, with only when the book was finished.
	e.do(e.owner, "PUT", "/api/readings/"+rid+"/note", map[string]any{"text": "Haunting.", "public": true})
	_, theirs = e.do(e.visitor, "GET", "/api/library", nil)
	notes, _ := theirs["books"].([]any)[0].(map[string]any)["notes"].([]any)
	if len(notes) != 1 || notes[0].(map[string]any)["text"] != "Haunting." || len(notes[0].(map[string]any)) != 2 {
		t.Fatalf("public notes: %v", notes)
	}
}

func TestSettingsPatchIsAllOrNothing(t *testing.T) {
	e := setup(t)
	if code, _ := e.do(e.owner, "PATCH", "/api/settings", map[string]any{"name": "Shelf", "wallDay": "nope"}); code != 422 {
		t.Fatalf("invalid patch: %d", code)
	}
	if code, _ := e.do(e.owner, "PATCH", "/api/settings", map[string]any{"name": "Shelf", "colour": "x"}); code != 422 {
		t.Fatalf("unknown field: %d", code)
	}
	if n := e.st.Snapshot().Settings.Name; n != "Bookshelf" {
		t.Fatalf("a rejected patch changed the name to %q", n)
	}
	code, st := e.do(e.owner, "PATCH", "/api/settings", map[string]any{"caseParts": map[string]any{"back": false}})
	if parts := st["caseParts"].(map[string]any); code != 200 || parts["back"] != false || parts["sides"] != true {
		t.Fatalf("partial nested patch: %d %v", code, st)
	}
}

func TestBackupExportImportRestore(t *testing.T) {
	e := setup(t)
	id := e.addBook("Austerlitz", "9780375756566", false)
	e.do(e.owner, "PUT", "/api/books/"+id+"/cover", png1(t))

	code, b := e.do(e.owner, "POST", "/api/backups", nil)
	if code != 201 || b["kind"] != "manual" || b["books"] != 1.0 {
		t.Fatalf("backup: %d %v", code, b)
	}
	name := b["name"].(string)

	// Export: CSV, JSON and a full archive.
	for _, f := range []string{"csv", "json", "archive"} {
		resp, _ := e.owner.Get(e.srv.URL + "/api/export?format=" + f)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") || len(body) == 0 {
			t.Fatalf("export %s: %d", f, resp.StatusCode)
		}
		if f == "csv" && !strings.Contains(string(body), "978-0-375-75656-6") {
			t.Fatalf("csv: %s", body)
		}
	}

	// Import a CSV: one new book, one already there.
	code, res := e.do(e.owner, "POST", "/api/import?name=books.csv", []byte("title,author,isbn\nAusterlitz,W. G. Sebald,9780375756566\nDune,Frank Herbert,9780441172719\n"))
	if code != 200 || res["added"] != 1.0 || res["skipped"] != 1.0 {
		t.Fatalf("import: %d %v", code, res)
	}
	if code, res := e.do(e.owner, "POST", "/api/import?name=books.csv", []byte("name,writer\nX,Y\n")); code != 422 {
		t.Fatalf("a CSV without titles: %d %v", code, res)
	}

	// Restore the backup: back to one book, with its cover.
	if code, res := e.do(e.owner, "POST", "/api/backups/"+name+"/restore", nil); code != 200 {
		t.Fatalf("restore: %d %v", code, res)
	}
	_, lib := e.do(e.owner, "GET", "/api/library", nil)
	books := lib["books"].([]any)
	if len(books) != 1 || books[0].(map[string]any)["cover"] == nil {
		t.Fatalf("after restore: %v", books)
	}
	cover := books[0].(map[string]any)["cover"].(map[string]any)["url"].(string)
	if resp, _ := e.owner.Get(e.srv.URL + cover); resp.StatusCode != 200 {
		t.Fatalf("the restored cover isn't served: %d", resp.StatusCode)
	}
	if code, _ := e.do(e.owner, "POST", "/api/backups/../../etc/restore", nil); code == 200 {
		t.Fatal("a path outside the backups was restored")
	}
	if code, _ := e.do(e.visitor, "GET", "/api/export?format=archive", nil); code != 401 {
		t.Fatal("a visitor exported the library")
	}
}

// The name may be Markdown: the page title and a phone's home screen get its words.
func TestAMarkdownName(t *testing.T) {
	e := setup(t)
	if code, st := e.do(e.owner, "PATCH", "/api/settings", map[string]any{"name": "[Our **books**](https://alog.lu)"}); code != 200 || st["name"] != "[Our **books**](https://alog.lu)" {
		t.Fatalf("name: %d %v", code, st)
	}
	resp, _ := e.visitor.Get(e.srv.URL + "/")
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(page), "<title>Our books</title>") {
		t.Fatalf("the page's title: %s", page)
	}
	resp, _ = e.visitor.Get(e.srv.URL + "/manifest.json")
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if m["name"] != "Our books" {
		t.Fatalf("the manifest's name: %v", m["name"])
	}
	if code, _ := e.do(e.owner, "PATCH", "/api/settings", map[string]any{"name": "[ ](https://alog.lu)"}); code != 422 {
		t.Fatalf("a name of only formatting: %d", code)
	}
	_, lib := e.do(e.visitor, "GET", "/api/library", nil)
	if st := lib["settings"].(map[string]any); st["footer"] != true || !strings.HasPrefix(st["footerText"].(string), "Made by [alog.lu]") || st["sortDesc"] != false {
		t.Fatalf("visitors' settings: %v", st)
	}
}
