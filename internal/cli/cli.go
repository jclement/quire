// Package cli implements the terminal verbs (`quire task add`, `quire
// search`, `quire today`) as thin clients over the HTTP API of a running
// quire — the same API the UI and agents use. Configure with QUIRE_URL
// (default http://localhost:8321) and QUIRE_TOKEN (unneeded in local
// no-auth mode).
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jclement/quire/internal/service"
)

type client struct {
	base  string
	token string
}

func newClient() *client {
	base := os.Getenv("QUIRE_URL")
	if base == "" {
		base = "http://localhost:8321"
	}
	return &client{base: strings.TrimRight(base, "/"), token: os.Getenv("QUIRE_TOKEN")}
}

func (c *client) do(method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("reaching quire at %s (set QUIRE_URL?): %w", c.base, err)
	}
	defer res.Body.Close()

	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("quire returned %s with an unreadable body", res.Status)
	}
	if envelope.Error != nil {
		return fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}

// Run dispatches a CLI verb. Returns an error suitable for printing.
func Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: quire task add <text> [--due <when>] [--defer <when>] | quire search <query> | quire today")
	}
	c := newClient()
	switch args[0] {
	case "add": // reached via `quire task add`
		return taskAdd(c, args[1:])
	case "search":
		return search(c, strings.Join(args[1:], " "))
	case "today":
		return today(c)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// ---- task add ----

func taskAdd(c *client, args []string) error {
	var words []string
	due, deferDate := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--due", "-d":
			i++
			if i >= len(args) {
				return fmt.Errorf("--due needs a value")
			}
			var err error
			if due, err = parseWhen(args[i]); err != nil {
				return err
			}
		case "--defer":
			i++
			if i >= len(args) {
				return fmt.Errorf("--defer needs a value")
			}
			var err error
			if deferDate, err = parseWhen(args[i]); err != nil {
				return err
			}
		default:
			words = append(words, args[i])
		}
	}
	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" {
		return fmt.Errorf("usage: quire task add <text> [--due <when>] [--defer <when>]")
	}

	var task struct {
		Text    string  `json:"text"`
		Due     *string `json:"due"`
		DocPath string  `json:"doc_path"`
	}
	err := c.do("POST", "/api/v1/tasks", map[string]string{"text": text, "due": due, "defer": deferDate}, &task)
	if err != nil {
		return err
	}
	line := "added: " + task.Text
	if task.Due != nil {
		line += "  (due " + *task.Due + ")"
	}
	fmt.Println(line)
	return nil
}

// parseWhen resolves a date the same way the server does — the parser lives
// in the service layer so every transport agrees on what "fri" means.
func parseWhen(s string) (string, error) {
	return service.ParseWhen(s, time.Now())
}

// ---- search ----

func search(c *client, query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("usage: quire search <query>")
	}
	var hits []struct {
		Path    string `json:"path"`
		Type    string `json:"type"`
		Title   string `json:"title"`
		Snippet string `json:"snippet"`
	}
	if err := c.do("GET", "/api/v1/search?q="+urlQuery(query), nil, &hits); err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("no results")
		return nil
	}
	for _, h := range hits {
		snippet := strings.NewReplacer("<mark>", "\x1b[1m", "</mark>", "\x1b[0m", "\n", " ").Replace(h.Snippet)
		fmt.Printf("%-8s %-40s %s\n", h.Type, h.Path, snippet)
	}
	return nil
}

// ---- today ----

func today(c *client) error {
	var payload struct {
		Date     string `json:"date"`
		Meetings []struct {
			Title string `json:"title"`
		} `json:"meetings"`
		Overdue  []cliTask `json:"overdue"`
		DueToday []cliTask `json:"due_today"`
		Waiting  []cliTask `json:"waiting"`
	}
	if err := c.do("GET", "/api/v1/today", nil, &payload); err != nil {
		return err
	}
	fmt.Println(payload.Date)
	printTasks := func(label string, tasks []cliTask) {
		if len(tasks) == 0 {
			return
		}
		fmt.Println("\n" + label + ":")
		for _, t := range tasks {
			due := ""
			if t.Due != nil {
				due = "  📅 " + *t.Due
			}
			fmt.Printf("  [ ] %s%s\n", t.Text, due)
		}
	}
	if len(payload.Meetings) > 0 {
		fmt.Println("\nmeetings:")
		for _, m := range payload.Meetings {
			fmt.Println("  •", m.Title)
		}
	}
	printTasks("overdue", payload.Overdue)
	printTasks("due today", payload.DueToday)
	printTasks("waiting", payload.Waiting)
	return nil
}

type cliTask struct {
	Text string  `json:"text"`
	Due  *string `json:"due"`
}

func urlQuery(s string) string {
	replacer := strings.NewReplacer(" ", "+", "&", "%26", "#", "%23", "?", "%3F", "=", "%3D")
	return replacer.Replace(s)
}
