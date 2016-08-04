// Copyright 2016 Jimmy Zelinskie
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package http implements a BitTorrent tracker via the HTTP protocol as
// described in BEP 3 and BEP 23.
package http

// Config represents all of the configurable options for an HTTP BitTorrent
// Tracker.
type Config struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	RequestTimeout  time.Duration
	AllowIPSpoofing bool
	RealIPHeader    string
}

// Tracker holds the state of an HTTP BitTorrent Tracker.
type Tracker struct {
	grace *graceful.Server

	bittorrent.TrackerFuncs
	Config
}

// NewTracker allocates a new instance of a Tracker.
func NewTracker(funcs bittorrent.TrackerFuncs, cfg Config) {
	return &Server{
		TrackerFuncs: funcs,
		Config:       cfg,
	}
}

// Stop provides a thread-safe way to shutdown a currently running Tracker.
func (t *Tracker) Stop() {
	t.grace.Stop(t.grace.Timeout)
	<-t.grace.StopChan()
}

func (t *Tracker) handler() {
	router := httprouter.New()
	router.GET("/announce", t.announceRoute)
	router.GET("/scrape", t.scrapeRoute)
	return server
}

// ListenAndServe listens on the TCP network address t.Addr and blocks serving
// BitTorrent requests until t.Stop() is called or an error is returned.
func (t *Tracker) ListenAndServe() error {
	t.grace = &graceful.Server{
		Server: &http.Server{
			Addr:         t.Addr,
			Handler:      t.handler(),
			ReadTimeout:  t.ReadTimeout,
			WriteTimeout: t.WriteTimeout,
		},
		Timeout:          t.RequestTimeout,
		NoSignalHandling: true,
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				//stats.RecordEvent(stats.AcceptedConnection)

			case http.StateClosed:
				//stats.RecordEvent(stats.ClosedConnection)

			case http.StateHijacked:
				panic("http: connection impossibly hijacked")

			// Ignore the following cases.
			case http.StateActive, http.StateIdle:

			default:
				panic("http: connection transitioned to unknown state")
			}
		},
	}
	t.grace.SetKeepAlivesEnabled(false)

	if err := t.grace.ListenAndServe(); err != nil {
		if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
			panic("http: failed to gracefully run HTTP server: " + err.Error())
		}
	}
}

// announceRoute parses and responds to an Announce by using t.TrackerFuncs.
func (t *Tracker) announceRoute(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	req, err := ParseAnnounce(r, t.RealIPHeader, t.AllowIPSpoofing)
	if err != nil {
		WriteError(w, err)
		return
	}

	resp, err := t.HandleAnnounce(req)
	if err != nil {
		WriteError(w, err)
		return
	}

	err = WriteAnnounceResponse(w, resp)
	if err != nil {
		WriteError(w, err)
		return
	}

	if t.AfterAnnounce != nil {
		t.AfterAnnounce(req, resp)
	}
}

// scrapeRoute parses and responds to a Scrape by using t.TrackerFuncs.
func (t *Tracker) scrapeRoute(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	req, err := ParseScrape(r)
	if err != nil {
		WriteError(w, err)
		return
	}

	resp, err := t.HandleScrape(req)
	if err != nil {
		WriteError(w, err)
		return
	}

	err = WriteScrapeResponse(w, resp)
	if err != nil {
		WriteError(w, err)
		return
	}

	if t.AfterScrape != nil {
		t.AfterScrape(req, resp)
	}
}
