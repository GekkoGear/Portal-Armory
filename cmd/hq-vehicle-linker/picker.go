package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GekkoGear/portal-armory/hqlink"
)

// picker shows the found vehicles and returns the checked ones per HQ ID.
// ok is false when the player cancelled.
type picker func(rep hqlink.Report, mapName string) (pick map[string][]string, ok bool)

//go:embed page.html
var pageHTML string

var pageTmpl = template.Must(template.New("page").Parse(pageHTML))

type pageVehicle struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Also string `json:"also"` // other HQs that already list it, comma separated
}

type pageHQ struct {
	ID       string        `json:"id"`
	Label    string        `json:"label"`
	Vehicles []pageVehicle `json:"vehicles"`
	Already  int           `json:"already"`
}

type pageData struct {
	Map    string   `json:"map"`
	HQs    []pageHQ `json:"hqs"`
	Others []string `json:"others"` // one line per HQ with nothing to pick
}

func buildPage(rep hqlink.Report, mapName string) pageData {
	d := pageData{Map: mapName}
	for _, h := range rep.HQs {
		if len(h.Found) == 0 {
			d.Others = append(d.Others, hqLabel(h)+": "+string(h.Status))
			continue
		}
		p := pageHQ{ID: h.ID, Label: hqLabel(h), Already: len(h.AlreadyLinked)}
		for _, c := range h.Found {
			p.Vehicles = append(p.Vehicles, pageVehicle{ID: c.SpawnerID, Type: c.VehicleType, Also: strings.Join(c.AlsoLinkedTo, ", ")})
		}
		d.HQs = append(d.HQs, p)
	}
	return d
}

type submission struct {
	Cancel bool                `json:"cancel"`
	Pick   map[string][]string `json:"pick"`
}

// pickServer serves the checklist page and waits for one submission.
type pickServer struct {
	token string
	page  pageData
	done  chan submission
	once  sync.Once
}

func newPickServer(rep hqlink.Report, mapName string) *pickServer {
	b := make([]byte, 16)
	rand.Read(b)
	return &pickServer{token: hex.EncodeToString(b), page: buildPage(rep, mapName), done: make(chan submission, 1)}
}

func (s *pickServer) handler() http.Handler {
	mux := http.NewServeMux()
	base := "/" + s.token + "/"
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		pageTmpl.Execute(w, s.page)
	})
	mux.HandleFunc("POST "+base+"submit", func(w http.ResponseWriter, r *http.Request) {
		// JSON only, so a plain form on some other web page can't post here.
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var sub submission
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&sub); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.once.Do(func() { s.done <- sub })
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	return mux
}

// browserPicker opens the checklist in the default browser. A line on cancel
// (the player pressing Enter in the console) cancels.
func browserPicker(cancel <-chan string, out io.Writer, open func(string) error) picker {
	return func(rep hqlink.Report, mapName string) (map[string][]string, bool) {
		s := newPickServer(rep, mapName)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprintf(out, "Couldn't start the checklist page: %v\n", err)
			return nil, false
		}
		srv := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
		go srv.Serve(ln)
		defer func() {
			ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
			defer stop()
			srv.Shutdown(ctx)
		}()

		url := fmt.Sprintf("http://%s/%s/", ln.Addr(), s.token)
		fmt.Fprintln(out, "The vehicle checklist is open in your browser.")
		fmt.Fprintln(out, "Check the vehicles you want, then press \"Add checked vehicles\".")
		if open(url) != nil {
			fmt.Fprintln(out, "Couldn't open the browser - open this link instead:")
		} else {
			fmt.Fprintln(out, "If it didn't open, go to:")
		}
		fmt.Fprintln(out, "  "+url)
		fmt.Fprintln(out, "(Press Enter here to cancel without changing anything.)")

		for {
			select {
			case sub := <-s.done:
				fmt.Fprintln(out)
				return sub.Pick, !sub.Cancel
			case _, open := <-cancel:
				if !open { // console input closed: keep waiting for the page
					cancel = nil
					continue
				}
				return nil, false
			}
		}
	}
}

// hqLabel is the HQ's name, with its id added when the two differ.
func hqLabel(h hqlink.HQResult) string {
	switch {
	case h.Name == "":
		return h.ID
	case h.Name == h.ID:
		return h.Name
	default:
		return h.Name + " (" + h.ID + ")"
	}
}
