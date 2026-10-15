package archive

import (
	"archive/zip"
	"bytes"
	"testing"
)

// A 2.0 archive kept one note on each finished reading; it opens with those
// notes as entries tied to their readings.
func TestOpensA20Archive(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"manifest.json": `{"kind":"bookshelf","format":1,"version":"2.0.1","books":1}`,
		"library.json": `{"books":[{"id":"b1","title":"Stoner"}],"readings":[
			{"id":"r1","bookId":"b1","outcome":"finished","note":{"text":"Better the second time.","public":true,"createdAt":"2024-03-12T10:00:00Z"}},
			{"id":"r2","bookId":"b1","outcome":"finished","note":null}]}`,
	} {
		f, _ := z.Create(name)
		f.Write([]byte(body))
	}
	z.Close()

	a, err := Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	es := a.Library.Entries
	if len(es) != 1 || es[0].Text != "Better the second time." || !es[0].Public || es[0].BookID != "b1" || es[0].ReadingID != "r1" {
		t.Fatalf("entries: %+v", es)
	}
	if len(a.Library.Readings) != 2 {
		t.Fatalf("readings: %+v", a.Library.Readings)
	}
}

// An early 2.1 archive (format 2) had notes and highlights with a page.
func TestOpensAFormat2Archive(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"manifest.json": `{"kind":"bookshelf","format":2,"version":"2.1.0-dev","books":1}`,
		"library.json": `{"books":[{"id":"b1","title":"Stoner"}],"entries":[
			{"bookId":"b1","kind":"note","text":"Sad.","public":true},
			{"bookId":"b1","kind":"highlight","quote":"A passage.","text":"Why.","page":33}]}`,
	} {
		f, _ := z.Create(name)
		f.Write([]byte(body))
	}
	z.Close()
	a, err := Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	es := a.Library.Entries
	if len(es) != 2 || es[0].Text != "Sad." || es[0].Quote != "" || es[1].Quote != "A passage." || es[1].Text != "Why." || es[1].Where == nil || es[1].Where.Kind != "page" || es[1].Where.N != 33 {
		t.Fatalf("entries: %+v", es)
	}
}
