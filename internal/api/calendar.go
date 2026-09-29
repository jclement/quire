// Calendar feed routes: managing the subscribed ICS feeds (Settings), the
// events themselves, making a meeting note from one, and meeting prep.
// Registered from Routes through calendarRoutes so the feature's surface
// reads in one place.
//
// Feed URLs are write-only: POST takes one, and every response after that
// carries the masked form. A client that needs the URL again gets it from
// the calendar provider, not from quire.
package api

import (
	"net/http"

	"github.com/jclement/quire/internal/service"
)

func (s *Server) calendarRoutes(mux Router) {
	mux.HandleFunc("GET /api/v1/calendar/feeds", s.handleListCalendarFeeds)
	mux.HandleFunc("POST /api/v1/calendar/feeds", s.handleAddCalendarFeed)
	mux.HandleFunc("DELETE /api/v1/calendar/feeds/{id}", s.handleRemoveCalendarFeed)
	mux.HandleFunc("POST /api/v1/calendar/refresh", s.handleRefreshCalendar)
	mux.HandleFunc("GET /api/v1/calendar/events", s.handleCalendarEvents)
	mux.HandleFunc("POST /api/v1/calendar/events/note", s.handleMeetingFromEvent)
	mux.HandleFunc("GET /api/v1/meeting-prep", s.handleMeetingPrep)
}

func (s *Server) handleListCalendarFeeds(w http.ResponseWriter, _ *http.Request) {
	feeds, err := s.Service.CalendarFeeds()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, feeds)
}

// handleAddCalendarFeed subscribes and fetches the new feed, waiting a few
// seconds so the list it answers with usually shows whether the URL worked;
// a slower feed answers "fetching" rather than holding the request open.
func (s *Server) handleAddCalendarFeed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	feeds, err := s.Service.AddCalendarFeed(r.Context(), body.URL)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, feeds)
}

func (s *Server) handleRemoveCalendarFeed(w http.ResponseWriter, r *http.Request) {
	removed, err := s.Service.RemoveCalendarFeed(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "no calendar feed with that id")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRefreshCalendar(w http.ResponseWriter, r *http.Request) {
	feeds, err := s.Service.RefreshCalendar(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, feeds)
}

func (s *Server) handleCalendarEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	events, err := s.Service.CalendarEvents(q.Get("from"), q.Get("to"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, events)
}

// handleMeetingFromEvent answers 201 with a new note, or 200 with the note
// the occurrence already has — the button is safe to press twice.
func (s *Server) handleMeetingFromEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UID  string `json:"uid"`
		Date string `json:"date"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	doc, created, err := s.Service.CreateMeetingFromEvent(body.UID, body.Date)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeData(w, status, doc)
}

// handleMeetingPrep takes ?path= (a meeting note) or ?event_uid= (with an
// optional &date= to pick the occurrence).
func (s *Server) handleMeetingPrep(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var (
		prep service.MeetingPrep
		err  error
	)
	switch {
	case q.Get("path") != "":
		prep, err = s.Service.MeetingPrep(q.Get("path"))
	case q.Get("event_uid") != "":
		prep, err = s.Service.MeetingPrepForEvent(q.Get("event_uid"), q.Get("date"))
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "pass path (a meeting note) or event_uid (a calendar event)")
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, prep)
}
