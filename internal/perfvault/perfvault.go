// Package perfvault writes a synthetic vault shaped like a heavy real one —
// people, companies, projects, meetings, two years of daily notes, and tens
// of thousands of tasks threaded with wikilinks and dates — so performance
// can be measured at the sizes DESIGN.md budgets for (10k docs / 50k tasks).
// It is deterministic: the same Config always writes the same files.
package perfvault

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config sizes the vault. Scale multiplies every count; Default is the
// "busy personal vault" shape (~5.5k documents, ~20k tasks), and Scale 2 is
// roughly DESIGN's 10k-document budget.
type Config struct {
	Notes, People, Companies, Projects, Meetings int
	// Days of daily notes, ending at Today.
	Days  int
	Today time.Time
	Seed  uint64
}

// Default is the 1x vault.
func Default(today time.Time) Config {
	return Config{Notes: 3000, People: 300, Companies: 100, Projects: 150, Meetings: 400, Days: 730, Today: today, Seed: 1}
}

// Scaled multiplies every count in c by n (days stay as they are past two
// years only by adding notes — a daily note per day is the real shape).
func (c Config) Scaled(n float64) Config {
	m := func(v int) int { return int(float64(v) * n) }
	c.Notes, c.People, c.Companies, c.Projects, c.Meetings = m(c.Notes), m(c.People), m(c.Companies), m(c.Projects), m(c.Meetings)
	c.Days = m(c.Days)
	return c
}

// Stats is what Generate wrote.
type Stats struct{ Docs, Tasks int }

var (
	firstNames = []string{"Sarah", "Dan", "Priya", "Marco", "Aiko", "Liam", "Fatima", "Noah", "Chen", "Olga", "Kwame", "Ines", "Tomas", "Mei", "Raj", "Lucia", "Omar", "Hana", "Ethan", "Zara"}
	lastNames  = []string{"Chen", "Roe", "Patel", "Rossi", "Tanaka", "Murphy", "Khan", "Smith", "Wei", "Petrova", "Mensah", "Garcia", "Novak", "Lin", "Shah", "Silva", "Haddad", "Kim", "Brown", "Ali"}
	words      = strings.Fields("pipeline budget forecast review rollout migration audit contract renewal hiring roadmap pricing launch backlog incident postmortem invoice onboarding security compliance design spec demo pilot integration metrics dashboard quarterly planning vendor proposal")
	verbs      = []string{"Follow up with", "Send notes to", "Ask", "Review draft from", "Call", "Book time with", "Chase", "Draft reply to", "Share deck with", "Confirm scope with"}
)

type gen struct {
	cfg   Config
	dir   string
	r     *rand.Rand
	stats Stats

	people, companies, projects []string // titles
}

// Generate writes the vault into dir (which should be empty).
func Generate(dir string, cfg Config) (Stats, error) {
	g := &gen{cfg: cfg, dir: dir, r: rand.New(rand.NewPCG(cfg.Seed, cfg.Seed*7919))}
	for i := range cfg.People {
		g.people = append(g.people, fmt.Sprintf("%s %s %d", firstNames[i%len(firstNames)], lastNames[(i/len(firstNames))%len(lastNames)], i))
	}
	for i := range cfg.Companies {
		g.companies = append(g.companies, fmt.Sprintf("%s %s Co %d", cap(words[i%len(words)]), cap(words[(i*7+3)%len(words)]), i))
	}
	for i := range cfg.Projects {
		g.projects = append(g.projects, fmt.Sprintf("Project %s %s %d", cap(words[(i*3)%len(words)]), cap(words[(i*11+5)%len(words)]), i))
	}
	steps := []func() error{g.writeCompanies, g.writePeople, g.writeProjects, g.writeMeetings, g.writeDaily, g.writeNotes}
	for _, step := range steps {
		if err := step(); err != nil {
			return g.stats, err
		}
	}
	return g.stats, nil
}

func cap(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

func slug(s string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (g *gen) write(rel, content string) error {
	p := filepath.Join(g.dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	g.stats.Docs++
	return os.WriteFile(p, []byte(content), 0o644)
}

func (g *gen) pick(list []string) string { return list[g.r.IntN(len(list))] }

// pickHot skews toward the front of the list: a few people and projects
// show up everywhere, which is what makes their pages heavy.
func (g *gen) pickHot(list []string) string {
	n := len(list)
	i := int(float64(n) * g.r.Float64() * g.r.Float64() * g.r.Float64())
	return list[min(i, n-1)]
}

func (g *gen) day(offset int) time.Time { return g.cfg.Today.AddDate(0, 0, -offset) }

func (g *gen) sentence() string {
	n := 6 + g.r.IntN(10)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = words[g.r.IntN(len(words))]
	}
	parts[0] = cap(parts[0])
	return strings.Join(parts, " ") + "."
}

func (g *gen) prose(lines int) string {
	var b strings.Builder
	for range lines {
		b.WriteString(g.sentence())
		if g.r.IntN(3) == 0 {
			fmt.Fprintf(&b, " See [[%s]].", g.pickHot(g.people))
		}
		if g.r.IntN(5) == 0 {
			fmt.Fprintf(&b, " Re [[%s]].", g.pickHot(g.projects))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// task writes one task line written on day `written` (an offset back from
// today). Older tasks are mostly done; recent ones are mostly open, with
// due/defer dates around today, waiting markers and priorities.
func (g *gen) task(written int) string {
	g.stats.Tasks++
	var b strings.Builder
	open := written < 21 || g.r.IntN(100) < 12
	if open {
		b.WriteString("- [ ] ")
	} else {
		b.WriteString("- [x] ")
	}
	who := g.pickHot(g.people)
	fmt.Fprintf(&b, "%s [[%s]] about %s", g.pick(verbs), who, words[g.r.IntN(len(words))])
	if g.r.IntN(3) == 0 {
		fmt.Fprintf(&b, " for [[%s]]", g.pickHot(g.projects))
	}
	if g.r.IntN(8) == 0 {
		fmt.Fprintf(&b, " #%s", words[g.r.IntN(len(words))])
	}
	if g.r.IntN(2) == 0 {
		due := g.day(written - g.r.IntN(30)).Format("2006-01-02")
		fmt.Fprintf(&b, " 📅 %s", due)
	} else if g.r.IntN(6) == 0 {
		fmt.Fprintf(&b, " 🛫 %s", g.day(written-g.r.IntN(20)).Format("2006-01-02"))
	}
	switch g.r.IntN(12) {
	case 0:
		b.WriteString(" ⏫")
	case 1:
		b.WriteString(" 🔼")
	case 2:
		b.WriteString(" 🔽")
	}
	if open && g.r.IntN(10) == 0 {
		fmt.Fprintf(&b, " ⏳ %s", g.day(written).Format("2006-01-02"))
	}
	if !open {
		fmt.Fprintf(&b, " ✅ %s", g.day(max(written-g.r.IntN(5), 0)).Format("2006-01-02"))
	}
	b.WriteString("\n")
	return b.String()
}

func (g *gen) tasks(written, n int) string {
	var b strings.Builder
	for range n {
		b.WriteString(g.task(written))
	}
	return b.String()
}

func (g *gen) writeCompanies() error {
	for _, c := range g.companies {
		body := fmt.Sprintf("---\ntype: company\n---\n# %s\n\n%s\n## Log\n\n%s", c, g.prose(3), g.prose(4))
		if err := g.write("companies/"+slug(c)+".md", body); err != nil {
			return err
		}
	}
	return nil
}

func (g *gen) writePeople() error {
	for i, p := range g.people {
		fm := fmt.Sprintf("type: person\ncompany: \"[[%s]]\"\n", g.pick(g.companies))
		if i%5 == 0 {
			fm += fmt.Sprintf("birthday: %s\n", g.day(g.r.IntN(365)).Format("01-02"))
		}
		body := fmt.Sprintf("---\n%s---\n# %s\n\n%s\n## 1:1 notes\n\n%s%s", fm, p, g.prose(2), g.prose(5), g.tasks(g.r.IntN(200), 2))
		if err := g.write("people/"+slug(p)+".md", body); err != nil {
			return err
		}
	}
	return nil
}

func (g *gen) writeProjects() error {
	statuses := []string{"planned", "active", "active", "on-hold", "done"}
	for _, p := range g.projects {
		fm := fmt.Sprintf("type: project\nstatus: %s\ncompany: \"[[%s]]\"\npeople: [\"[[%s]]\", \"[[%s]]\"]\n",
			g.pick(statuses), g.pick(g.companies), g.pickHot(g.people), g.pickHot(g.people))
		var b strings.Builder
		for k := range 10 {
			b.WriteString(g.task(g.r.IntN(400) * (k%2 + 1) / 2))
		}
		body := fmt.Sprintf("---\n%s---\n# %s\n\n## Goal\n\n%s\n## Tasks\n\n%s\n## Notes\n\n%s", fm, p, g.prose(3), b.String(), g.prose(6))
		if err := g.write("projects/"+slug(p)+".md", body); err != nil {
			return err
		}
	}
	return nil
}

func (g *gen) writeMeetings() error {
	span := max(g.cfg.Days, 1)
	for i := range g.cfg.Meetings {
		off := (i * span) / max(g.cfg.Meetings, 1)
		// A couple land today so Today has meetings to show.
		if i >= g.cfg.Meetings-2 {
			off = 0
		}
		d := g.day(off)
		title := fmt.Sprintf("%s sync %d", cap(words[i%len(words)]), i)
		fm := fmt.Sprintf("type: meeting\ndate: %sT%02d:00\npeople: [\"[[%s]]\", \"[[%s]]\", \"[[%s]]\"]\nproject: \"[[%s]]\"\ncompany: \"[[%s]]\"\n",
			d.Format("2006-01-02"), 9+i%8, g.pickHot(g.people), g.pickHot(g.people), g.pick(g.people), g.pickHot(g.projects), g.pick(g.companies))
		body := fmt.Sprintf("---\n%s---\n# %s\n\n## Notes\n\n%s\n## Decisions\n\n- %s\n- %s\n\n## Action Items\n\n%s",
			fm, title, g.prose(8), g.sentence(), g.sentence(), g.tasks(off, 5))
		if err := g.write("meetings/"+d.Format("2006-01-02")+"-"+slug(title)+".md", body); err != nil {
			return err
		}
	}
	return nil
}

func (g *gen) writeDaily() error {
	for off := range g.cfg.Days {
		d := g.day(off)
		body := fmt.Sprintf("# %s\n\n## Log\n\n%s\n## Tasks\n\n%s", d.Format("Monday, January 2, 2006"), g.prose(6), g.tasks(off, 10))
		if err := g.write("daily/"+d.Format("2006-01-02")+".md", body); err != nil {
			return err
		}
	}
	return nil
}

func (g *gen) writeNotes() error {
	for i := range g.cfg.Notes {
		off := g.r.IntN(max(g.cfg.Days, 1))
		title := fmt.Sprintf("%s %s note %d", cap(words[g.r.IntN(len(words))]), words[g.r.IntN(len(words))], i)
		fm := fmt.Sprintf("tags: [%s]\n", words[g.r.IntN(len(words))])
		if i%4 == 0 {
			fm += fmt.Sprintf("project: \"[[%s]]\"\n", g.pickHot(g.projects))
		}
		body := fmt.Sprintf("---\n%s---\n# %s\n\n%s\n%s\n%s", fm, title, g.prose(10), g.tasks(off, 3), g.prose(4))
		rel := "notes/" + slug(title) + ".md"
		if err := g.write(rel, body); err != nil {
			return err
		}
		mt := g.day(off)
		_ = os.Chtimes(filepath.Join(g.dir, rel), mt, mt)
	}
	return nil
}

// HotPersonTitle is the person who appears most often (pickHot skews to the
// front of the list), the heaviest person page in the vault.
func HotPersonTitle(c Config) string {
	return fmt.Sprintf("%s %s %d", firstNames[0], lastNames[0], 0)
}

// HotPersonSlug is HotPersonTitle's file name stem.
func HotPersonSlug(c Config) string { return slug(HotPersonTitle(c)) }

// HotProjectSlug is the most-referenced project's file name stem.
func HotProjectSlug(c Config) string {
	return slug(fmt.Sprintf("Project %s %s %d", cap(words[0]), cap(words[5]), 0))
}
