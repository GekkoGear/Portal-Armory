package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GekkoGear/portal-armory/hqlink"
)

func findFixture(t *testing.T) hqlink.Report {
	t.Helper()
	b, err := os.ReadFile("../../hqlink/testdata/small_pretty.json")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := hqlink.Find(b)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestBuildPage(t *testing.T) {
	d := buildPage(findFixture(t), "Small.spatial.json")
	if len(d.HQs) != 4 || d.HQs[1].ID != "HQ_B" || len(d.HQs[1].Vehicles) != 3 || d.HQs[1].Vehicles[2].Type != "RHIB" {
		t.Fatalf("page HQs = %+v", d.HQs)
	}
	if d.HQs[0].Already != 1 {
		t.Errorf("HQ_A already = %d", d.HQs[0].Already)
	}
	if len(d.Others) != 3 { // HQ_Empty, HQ_NoArea, HQ_BadArea
		t.Errorf("others = %v", d.Others)
	}
}

func TestPageServer(t *testing.T) {
	s := newPickServer(findFixture(t), `Map "<x>".spatial.json`)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	base := srv.URL + "/" + s.token + "/"

	r, _ := http.Get(base)
	body, _ := io.ReadAll(r.Body)
	page := string(body)
	if r.StatusCode != 200 || !strings.Contains(page, `"HQ_B"`) || !strings.Contains(page, "Add checked vehicles") {
		t.Fatalf("page: %d", r.StatusCode)
	}
	if strings.Contains(page, `"<x>"`) {
		t.Error("map name not escaped in the page")
	}
	if r, _ := http.Get(srv.URL + "/wrongtoken/"); r.StatusCode != 404 {
		t.Errorf("wrong token = %d", r.StatusCode)
	}
	if r, _ := http.Post(base+"submit", "application/x-www-form-urlencoded", strings.NewReader("a=b")); r.StatusCode != 400 {
		t.Errorf("form post = %d", r.StatusCode)
	}
	sub := `{"cancel":false,"pick":{"HQ_B":["VB1"]}}`
	if r, _ := http.Post(base+"submit", "application/json", strings.NewReader(sub)); r.StatusCode != 200 {
		t.Errorf("submit = %d", r.StatusCode)
	}
	got := <-s.done
	if got.Cancel || !reflect.DeepEqual(got.Pick, map[string][]string{"HQ_B": {"VB1"}}) {
		t.Errorf("submission = %+v", got)
	}
}

func TestBrowserPickerSubmit(t *testing.T) {
	var out bytes.Buffer
	open := func(url string) error {
		go func() {
			body, _ := json.Marshal(submission{Pick: map[string][]string{"HQ_A": {"VA2"}}})
			http.Post(url+"submit", "application/json", bytes.NewReader(body))
		}()
		return nil
	}
	pick, ok := browserPicker(make(chan string), &out, open)(findFixture(t), "x")
	if !ok || !reflect.DeepEqual(pick, map[string][]string{"HQ_A": {"VA2"}}) {
		t.Errorf("pick=%v ok=%v", pick, ok)
	}
	if !strings.Contains(out.String(), "http://127.0.0.1:") {
		t.Error("link not printed")
	}
}

func TestBrowserPickerCancelButtonAndEnter(t *testing.T) {
	var out bytes.Buffer
	open := func(url string) error {
		go http.Post(url+"submit", "application/json", strings.NewReader(`{"cancel":true}`))
		return nil
	}
	if _, ok := browserPicker(make(chan string), &out, open)(findFixture(t), "x"); ok {
		t.Error("cancel button should cancel")
	}
	lines := make(chan string, 1)
	lines <- ""
	failOpen := func(string) error { return io.EOF }
	done := make(chan bool)
	go func() {
		_, ok := browserPicker(lines, &out, failOpen)(findFixture(t), "x")
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Error("Enter should cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Enter didn't cancel")
	}
	if !strings.Contains(out.String(), "open this link instead") {
		t.Error("no fallback link message when the browser can't open")
	}
}
