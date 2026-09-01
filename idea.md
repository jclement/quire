Yes. I’d make this deliberately **not “Obsidian in a browser.”** The opinionated data model is the interesting part: Markdown remains the durable storage format, but People / Companies / Projects / Meetings / Tasks are first-class concepts rather than conventions you have to invent.

I’d also design it from day one so an AI agent can operate the whole thing through the same API/MCP surface as the UI. That could be a killer feature.

Here’s the direction I’d spec:

- **Go backend**, single binary
- **React + TypeScript + Tailwind**
- **mise** for toolchain/dev environment
- SQLite for application/index/auth state
- **Markdown + attachments on disk are canonical**
- Single-user by default; multi-user is a configuration/deployment choice
- Passkeys/WebAuthn as primary human auth
- OAuth 2.1 for MCP; bearer/API tokens for automation
- Excellent REST/JSON API
- First-class MCP server
- Docker/Compose deployment
- No required external services

# Personal Knowledge & Work Hub — Product Specification

## 1. Product Vision

Build a beautiful, fast, self-hostable personal knowledge and work management application combining the strongest ideas from:

- Obsidian
- Notion
- Things / Todoist
- Personal CRM systems
- Meeting-note tools
- Lightweight project management
- Modern Markdown editors

The application is intentionally more opinionated than Obsidian.

Markdown files remain the durable, human-readable source of truth, but the application understands important real-world concepts:

**Notes · People · Companies · Meetings · Projects · Tasks**

The goal is not simply to store notes.

The goal is to answer:

> What am I working on, who is involved, what happened, what did I promise, and what should I do next?

The system should work equally well for a single individual, a small team, and AI agents.

---

# 2. Core Principles

## Markdown First

User-authored knowledge is stored as ordinary Markdown files on disk.

The vault remains useful without the application.

A user must be able to:

- open files in another editor
- grep them
- edit them with Vim/VS Code
- put the entire repository in Git
- back it up with ordinary filesystem tools
- migrate away without an export process

Application-specific metadata uses understandable YAML front matter.

Example:

```markdown
---
type: person
id: person_sarah_chen
company: "[[Acme Corp]]"
email: sarah@example.com
tags:
  - customer
---

# Sarah Chen

Sarah runs infrastructure at [[Acme Corp]].

## Notes

Interested in moving reporting workloads to Postgres.
```

The application maintains indexes and caches but Markdown content must never be trapped in the database.

---

# 3. Architecture

## Backend

Go.

Prefer a straightforward modular monolith over microservices.

Primary responsibilities:

- HTTP API
- authentication
- authorization
- Markdown filesystem management
- metadata extraction
- search/indexing
- task engine
- sharing
- attachment management
- email
- MCP server
- OAuth authorization server
- realtime UI notifications where useful

The server should compile to a single executable.

---

## Frontend

React + TypeScript + Tailwind CSS.

Recommended supporting stack:

- Vite
- TanStack Router
- TanStack Query
- CodeMirror 6
- Radix/shadcn-style accessible UI primitives

The interface should feel like a polished native productivity application rather than an admin dashboard.

Responsive design is mandatory.

Desktop, tablet and phone are all first-class targets.

---

## Development Environment

Use `mise` as the canonical development tool/environment manager.

A new developer should ideally be able to run:

```bash
git clone ...
cd app
mise install
mise run dev
```

Useful tasks:

```bash
mise run dev
mise run test
mise run lint
mise run build
mise run docker
```

---

# 4. Storage Model

Use two storage layers.

## Filesystem — Canonical User Content

Example:

```text
data/
  notes/
  people/
  companies/
  projects/
  meetings/
  daily/
  attachments/
```

Do not require this exact directory structure internally, but provide it as the clean default.

Example:

```text
people/
  sarah-chen.md

companies/
  acme.md

projects/
  reporting-redesign.md

meetings/
  2026-08-31-acme-reporting.md
```

---

## SQLite — Application State

SQLite stores derived and operational information such as:

- users
- credentials
- passkeys
- sessions
- OAuth clients
- OAuth grants
- API tokens
- parsed document metadata
- links/backlinks
- search index
- task index
- attachment metadata
- sharing tokens
- notification state
- email delivery records

SQLite should be considered rebuildable wherever practical.

Deleting the database must never destroy the user's knowledge base.

A rebuild command should reconstruct indexes from Markdown.

```bash
app reindex
```

---

# 5. Documents

Every Markdown document can contain:

- YAML front matter
- Markdown
- wiki links
- Markdown links
- tags
- tasks
- images
- attachments
- Mermaid diagrams
- fenced code blocks
- callouts

Support:

```markdown
[[Project Apollo]]
[[Sarah Chen]]
[[Project Apollo|Apollo]]
```

Standard Markdown links must also work.

---

# 6. First-Class Entity Types

## Notes

General-purpose Markdown documents.

## People

People have structured properties including:

- name
- aliases
- company
- title
- email
- phone
- tags
- important dates
- related projects
- last interaction

Person pages automatically surface:

- recent meetings
- projects
- tasks
- notes mentioning the person
- companies
- backlinks

The user should not have to manually construct a personal CRM using plugins and Dataview queries.

---

## Companies

Company pages include:

- name
- domain
- website
- contacts
- tags
- notes
- projects
- meetings
- outstanding tasks

---

## Projects

Projects are Markdown documents with structured metadata.

Suggested properties:

```yaml
---
type: project
status: active
owner: "[[Jeff]]"
people:
  - "[[Sarah Chen]]"
company: "[[Acme]]"
due: 2026-10-01
---
```

Project pages automatically aggregate:

- notes
- people
- meetings
- tasks
- files
- recent activity

Projects support at least:

- active
- waiting
- someday
- completed
- archived

---

## Meetings

Meetings are first-class documents.

Suggested front matter:

```yaml
---
type: meeting
date: 2026-08-31T14:00
people:
  - "[[Sarah Chen]]"
project: "[[Reporting Redesign]]"
company: "[[Acme]]"
---
```

Meeting UI provides dedicated sections for:

- attendees
- agenda
- notes
- decisions
- action items

Tasks written during a meeting become real tasks.

Example:

```markdown
- [ ] Send Sarah architecture diagram 📅 2026-09-02
```

The task automatically retains provenance:

> Created in "Acme Reporting Meeting — Aug 31"

---

# 7. Task Management

Tasks are a core product feature, not a Markdown checkbox viewer.

Tasks may originate inside any Markdown document.

Support:

- title
- completed
- due date
- start/defer date
- priority
- project
- assignee
- person/company relationship
- tags
- recurring tasks
- waiting-for
- notes
- source document
- creation/completion timestamps

Provide dedicated views:

**Inbox**

Unprocessed tasks.

**Today**

Available tasks relevant today.

**Upcoming**

Future tasks grouped by date.

**Waiting**

Things delegated or awaiting someone else.

**Projects**

Tasks grouped by project.

**Someday**

Deferred ideas.

The application should support a Things-like workflow without forcing every user to adopt it.

Tasks embedded in Markdown and task-management views must remain synchronized.

---

# 8. Markdown Editor

Editing quality is critical.

Use CodeMirror 6 or an equivalent high-quality editing foundation.

Support syntax coloring for:

- headings
- bold/italic
- links
- wiki links
- tags
- YAML
- code
- task syntax
- callouts

Modes:

**Edit**

Full-screen Markdown editing.

**Split**

Markdown source on the left, rendered document on the right.

**Read**

Beautiful rendered document.

On mobile, Edit and Read should switch cleanly rather than attempting an unusably narrow split view.

---

# 9. Markdown Extensions

## Mermaid

````markdown
```mermaid
graph TD
    A --> B
```
````

````

Render diagrams safely.

---

## Syntax Highlighting

Support common programming languages in fenced code blocks.

Include a copy button.

---

## Callouts

Support Obsidian-compatible syntax where reasonable:

```markdown
> [!NOTE]
> Important information.
````

Types include:

- note
- info
- tip
- warning
- danger
- question
- success
- example

---

# 10. Images and Attachments

Images must be painless.

Support:

- clipboard paste
- drag/drop
- file picker
- mobile photo upload

Pasting an image into the editor should immediately save it into the attachment store and insert appropriate Markdown.

Example:

```markdown
![[attachments/2026/08/image-a83f91.png]]
```

Prefer collision-resistant generated filenames while retaining the original filename as metadata where useful.

Images should support:

- preview
- lightbox
- download
- copy link
- delete when unreferenced

Never silently delete attachments merely because a Markdown reference disappears.

---

# 11. Search

Search should feel instantaneous.

Search:

- titles
- body text
- tags
- people
- companies
- projects
- meetings
- tasks

Support filters such as:

```text
type:meeting sarah
project:apollo
tag:security
is:task due:today
company:acme
```

SQLite FTS5 is a good initial implementation.

Avoid requiring Elasticsearch/OpenSearch.

---

# 12. Backlinks and Relationships

Every document automatically exposes:

**Links**

Documents referenced by this document.

**Backlinks**

Documents referencing this document.

**Related**

Relationships inferred from structured metadata.

The application should make relationships useful without turning the UI into a graph visualization experiment.

A graph view may exist, but it is secondary.

---

# 13. Authentication

## Human Authentication

Passkeys/WebAuthn are the preferred authentication mechanism.

Passwordless operation should be fully supported.

Recovery mechanisms should be explicitly designed rather than quietly falling back to weak password authentication.

Single-user deployments should have an extremely simple bootstrap process.

---

# 14. Single-User and Multi-User Modes

The same application supports both.

Example:

```yaml
mode: single-user
```

or:

```yaml
mode: multi-user
```

Single-user mode removes unnecessary collaboration UI.

Multi-user mode adds:

- user accounts
- ownership
- permissions
- task assignment
- private/shared documents
- shared projects

Do not complicate the single-user experience merely because multi-user support exists.

---

# 15. API

Everything meaningful the web UI can do should have a supported API.

The UI should use substantially the same API.

Example resources:

```text
/api/v1/notes
/api/v1/people
/api/v1/companies
/api/v1/projects
/api/v1/meetings
/api/v1/tasks
/api/v1/search
/api/v1/attachments
/api/v1/share
```

Provide an OpenAPI specification.

Automation should never require manipulating undocumented internal database structures.

---

# 16. API Tokens

Support scoped bearer tokens for scripts and integrations.

Example scopes:

```text
notes:read
notes:write
tasks:read
tasks:write
people:read
projects:read
search:read
```

Tokens should support:

- names
- creation dates
- expiry
- revocation
- last-used timestamp

---

# 17. MCP

MCP is a first-class interface rather than a later plugin.

Expose useful tools such as:

```text
search
get_document
create_note
update_note

list_people
get_person
create_person

list_projects
get_project
create_project
update_project

list_tasks
create_task
update_task
complete_task

create_meeting
get_meeting
```

Also expose appropriate MCP resources.

An AI assistant should be able to answer questions such as:

> What did Sarah and I decide about reporting?

> What am I waiting for from Acme?

> Create a project for migrating the reporting infrastructure.

> Add these four action items to today's meeting.

> What should I work on today?

The MCP interface must obey exactly the same permissions as the HTTP API.

---

# 18. MCP Authentication

Support modern OAuth-based MCP authentication.

Also allow scoped bearer tokens for controlled integrations.

OAuth configuration should support remote MCP clients without requiring users to manually copy long-lived secrets around.

Security-sensitive authentication logic should rely on mature libraries wherever practical.

---

# 19. Sharing

Documents can generate external sharing links.

Sharing defaults to disabled.

A share link may support:

- read-only access
- expiration
- optional passcode
- revocation
- view count
- optional attachment access

Example conceptual URL:

```text
https://notes.example.com/s/a8H3kd92
```

Shared pages should look excellent and not expose the internal application interface.

Projects and meeting notes should be particularly pleasant to share.

---

# 20. Email

Email delivery is built in.

SMTP configuration should be sufficient.

Use cases:

- email a note
- email meeting notes
- email a shared-link invitation
- task reminders
- daily task digest
- authentication/security notifications

Support both:

**Send rendered content**

and

**Send a link**

Email should be an application service with an abstraction allowing future providers such as SES, Postmark or Resend without redesigning the product.

---

# 21. Daily Notes

Daily notes are built in.

Example:

```text
daily/2026-08-31.md
```

The Today screen combines:

- today's note
- today's tasks
- overdue tasks
- meetings
- recently edited documents

This should become a natural application home screen.

---

# 22. Command Palette

Desktop:

`Cmd/Ctrl + K`

Provides fast access to:

- documents
- people
- projects
- commands
- task creation
- meeting creation
- search

Example commands:

```text
New Note
New Person
New Meeting
New Project
Add Task
Open Today
Search Everything
```

Keyboard-first operation should be excellent without compromising mobile usability.

---

# 23. Quick Capture

A universal "+" action should allow:

- Note
- Task
- Meeting
- Person
- Project

Mobile quick capture should be exceptionally fast.

Creating a task should take only a few taps.

---

# 24. Home / Today

The default home screen should answer:

> What matters right now?

Suggested layout:

**Today**

Meetings occurring today.

**Must Do**

High-priority/due tasks.

**Tasks**

Remaining available tasks.

**Waiting For**

Items requiring follow-up.

**Recent**

Recently edited notes/projects.

**Daily Note**

Immediate writing/capture area.

Avoid turning Home into a customizable dashboard-building system in the initial product.

Opinionated defaults are preferable.

---

# 25. Navigation

Desktop sidebar:

```text
Today
Inbox
Tasks

Notes
People
Companies
Projects
Meetings

Daily Notes
Attachments

Search
```

Favorites/pinned items appear beneath the primary navigation.

Mobile uses a compact bottom navigation plus global quick-create action.

---

# 26. Visual Design

The product should feel:

- calm
- extremely fast
- polished
- dense without feeling crowded
- typography-focused
- professional
- subtly playful

Avoid the appearance of:

- enterprise CRUD software
- Bootstrap admin templates
- developer documentation sites
- generic Tailwind dashboards

Markdown rendering should be exceptionally attractive.

Support:

- light mode
- dark mode
- system mode

Responsive behavior must be designed intentionally rather than obtained accidentally through CSS wrapping.

---

# 27. Filesystem Watching

Because Markdown is intentionally editable outside the application, filesystem changes must be detected.

When a file changes externally:

1. detect change
2. parse front matter
3. update index
4. update backlinks
5. update tasks
6. update search index
7. notify connected UI clients if necessary

Application writes should be atomic.

Conflicts must never silently destroy user content.

---

# 28. Git Friendliness

Git integration should not be mandatory.

However, files should naturally behave well in Git.

Avoid:

- meaningless metadata churn
- rewriting entire documents unnecessarily
- volatile timestamps in Markdown unless useful
- application-generated binary state mixed into note directories

Optional future Git functionality may include:

- automatic commits
- history browser
- diff view
- restore version

---

# 29. Backup and Portability

A complete backup consists principally of:

```text
data/
```

plus configuration/secrets as appropriate.

Provide commands such as:

```bash
app backup
app reindex
app doctor
```

A user should have confidence that even if this application disappears tomorrow, their notes remain ordinary Markdown.

---

# 30. Deployment

Official Docker image.

Example deployment:

```yaml
services:
  app:
    image: ghcr.io/example/app:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      APP_BASE_URL: https://notes.example.com
```

Support reverse proxies cleanly:

- Traefik
- Caddy
- nginx
- Cloudflare Tunnel

No Redis, Postgres, Elasticsearch or message broker should be required for a normal deployment.

SQLite + filesystem should comfortably run the standard product.

---

# 31. Configuration

Configuration via environment variables and/or a readable config file.

Example:

```yaml
server:
  base_url: https://notes.example.com

auth:
  mode: multi-user
  passkeys: true

storage:
  path: /data

email:
  transport: smtp

features:
  sharing: true
  registration: false
```

Secrets should also support environment/file-based injection appropriate for Docker secrets.

---

# 32. AI / Agent Design Principle

AI is not required to use the application.

The application itself should remain deterministic and useful without an LLM.

However, it should be extraordinarily AI-friendly.

An agent can:

- search everything
- inspect relationships
- create/update notes
- manage projects
- create and complete tasks
- prepare meetings
- summarize previous meetings
- identify outstanding commitments

This is accomplished through the API and MCP rather than by giving an agent unrestricted filesystem access.

Future AI features can therefore be layered on without corrupting the core product.

---

# 33. Recommended MVP

The first genuinely useful release should contain:

### Foundation

- Go server
- I’d keep the MVP fairly aggressive: enough to prove the integrated workflow, but resist turning v1 into an endless platform build.

# 33. Recommended MVP

The first genuinely useful release should contain:

### Foundation

- Go server
- React + TypeScript frontend
- Tailwind CSS
- `mise` development environment
- SQLite application database
- Markdown filesystem storage
- Filesystem watcher
- Docker image
- Docker Compose example
- Environment/config-file configuration
- Database migrations
- Structured logging
- Health/readiness endpoints

### Authentication

- Single-user mode
- Multi-user mode
- Passkey/WebAuthn registration and login
- Secure session management
- Bootstrap/admin-user creation
- Logout and passkey management
- Scoped bearer/API tokens

Do not require passwords for normal authentication.

### Markdown

- Create, read, update, rename and delete Markdown documents
- YAML front matter
- Standard Markdown
- Wiki links
- Backlinks
- Tags
- Git-friendly filesystem layout
- Atomic file writes
- Detection and reindexing of externally modified files

### Editor

- CodeMirror 6 Markdown editor
- Markdown syntax coloring
- Edit mode
- Split edit/preview mode
- Read mode
- Full-screen editing
- Keyboard shortcuts
- Unsaved-change protection
- Responsive mobile editing

The editor is a major part of the product experience and should feel excellent in the MVP rather than being treated as temporary scaffolding.

### Rich Markdown

Support from the first release:

- fenced code blocks
- syntax highlighting
- Mermaid
- tables
- task lists
- Obsidian-style callouts
- wiki links
- standard Markdown links
- images
- attachments

### Images and Attachments

- Paste images directly from clipboard
- Drag and drop files
- File picker
- Mobile image/photo upload
- Automatic attachment storage
- Markdown reference insertion
- Image previews
- Full-size image viewer
- Attachment downloads
- Detection of unreferenced attachments

Pasting a screenshot into a note should feel instantaneous.

### First-Class Entities

Implement these six types:

- Note
- Person
- Company
- Project
- Meeting
- Task

All except Task are represented primarily by Markdown documents.

Tasks may exist inside Markdown documents while also being indexed into the application's task system.

### People

Person pages should automatically show:

- contact metadata
- company
- related projects
- recent meetings
- open tasks
- notes mentioning the person
- backlinks

### Companies

Company pages should automatically show:

- people
- projects
- meetings
- tasks
- related notes

### Projects

Projects support:

- name
- description
- status
- due date
- people
- company
- tags

Project pages automatically aggregate:

- tasks
- meetings
- people
- notes
- backlinks

### Meetings

Meeting documents support:

- date/time
- attendees
- company
- project
- agenda
- notes
- decisions
- action items

Tasks created inside meeting notes must appear automatically in task views while retaining a link to the source meeting.

### Task Management

The MVP needs genuine task management rather than merely rendering Markdown checkboxes.

Support:

- create
- edit
- complete/reopen
- delete
- due date
- start/defer date
- priority
- project
- related person
- related company
- tags
- source document
- basic recurrence

Provide dedicated views for:

- Inbox
- Today
- Upcoming
- Waiting
- Someday
- Completed

Tasks embedded in Markdown and dedicated task views remain synchronized.

### Today

Build an opinionated Today screen containing:

- today's daily note
- overdue tasks
- tasks due today
- available tasks
- today's meetings
- waiting/follow-up items
- recently edited documents

Today should be useful enough to serve as the application's default screen.

### Daily Notes

- Automatic daily-note creation
- Date-based navigation
- Previous/next day
- Calendar picker
- Daily-note template
- Integration with Today

### Search

Use SQLite FTS5 initially.

Search across:

- titles
- Markdown contents
- people
- companies
- projects
- meetings
- tasks
- tags

Support useful filters including:

```text
type:person sarah
type:meeting reporting
project:apollo
company:acme
tag:security
is:task
due:today
```

Search should be accessible globally and feel effectively instantaneous for a normal personal knowledge base.

### Command Palette

`Cmd/Ctrl + K`

Search documents and execute commands.

Initial commands include:

```text
New Note
New Task
New Person
New Company
New Project
New Meeting
Open Today
Open Daily Note
Search
```

### Quick Capture

Provide a persistent quick-create action for:

- task
- note
- meeting
- person
- project

On mobile, task and note capture should require minimal interaction.

### API

Provide a versioned JSON API covering all core functionality.

At minimum:

```text
/api/v1/notes
/api/v1/people
/api/v1/companies
/api/v1/projects
/api/v1/meetings
/api/v1/tasks
/api/v1/search
/api/v1/attachments
/api/v1/shares
```

Publish an OpenAPI specification.

The React application should consume this API rather than relying on a separate privileged internal interface.

### MCP

Ship MCP in the MVP.

Provide tools covering at least:

```text
search
get_document
create_note
update_note

list_people
get_person
create_person

list_companies
get_company

list_projects
get_project
create_project
update_project

list_meetings
get_meeting
create_meeting

list_tasks
get_task
create_task
update_task
complete_task
```

The goal is that an MCP-connected assistant can perform essentially the same knowledge-management operations as a human using the web application.

### MCP/API Authentication

MVP support:

- scoped bearer tokens
- OAuth 2.1 authorization for remote MCP clients

Authorization scopes should be shared between MCP and the HTTP API where practical.

### Sharing

Provide revocable read-only sharing links for individual documents.

MVP controls:

- create link
- revoke link
- optional expiration
- optional attachment access

Shared documents should render as clean standalone pages without exposing the authenticated application UI.

### Email

Support SMTP delivery.

MVP email functionality:

- email a rendered note
- email meeting notes
- email a sharing link
- basic transactional/security email

Task reminder and digest systems can follow after MVP unless required for a specific deployment.

### Responsive UI

Desktop and mobile must both be production-quality.

Desktop should emphasize:

- sidebar navigation
- keyboard navigation
- split editor
- information density

Mobile should emphasize:

- Today
- quick capture
- tasks
- search
- reading
- straightforward Markdown editing

Do not attempt to reproduce the desktop layout on a small screen.

### Appearance

Ship with:

- light mode
- dark mode
- system mode
- excellent Markdown typography
- responsive tables/code blocks
- accessible keyboard focus
- accessible contrast
- reduced-motion support

The MVP should already look like a product someone wants to use every day.

### Operations

Provide:

```bash
app serve
app reindex
app doctor
app backup
```

Include:

- graceful shutdown
- SQLite backup support
- index rebuilding
- filesystem consistency checks
- configurable logging
- health checks
- automatic schema migrations

### Import

Provide a simple "existing Markdown vault" import path.

The application should be capable of pointing at or importing an existing directory containing Markdown and attachments.

At minimum preserve:

- Markdown
- directory structure
- YAML front matter
- attachments
- wiki links

Perfect Obsidian compatibility is not required for MVP.

Preserving users' files is.

---

# 34. Explicitly Not MVP

Avoid delaying the first release for:

- collaborative realtime editing
- comments
- elaborate role-based permissions
- native mobile applications
- browser extensions
- graph visualization
- plugin marketplace
- custom JavaScript plugins
- customizable database/table builders
- arbitrary user-defined entity schemas
- calendar synchronization
- Gmail/Outlook synchronization
- automatic meeting transcription
- embedded local LLMs
- vector databases
- semantic/AI search
- Git UI
- automatic Git commits
- complex workflow automation
- dashboards/dashboard builders
- themes/theme marketplace
- end-to-end encrypted synchronization
- hosted SaaS billing

These are potential later features, not prerequisites for proving the product.

---

# 35. MVP Success Criteria

The MVP succeeds if one person can deploy the application with Docker and comfortably use it as their primary personal work system.

A user should be able to start their morning by opening **Today**, see what needs attention, write their daily note, prepare for meetings, capture tasks during those meetings, associate the work with people/projects/companies, and later find exactly what happened.

They should be able to paste screenshots, write technical notes containing code and Mermaid diagrams, and share selected notes externally.

They should be able to edit the same Markdown files outside the application without breaking the system.

An AI assistant connected through MCP should be able to answer:

> What am I supposed to do today?

> What are my open tasks for Acme?

> When did I last talk to Sarah and what did we discuss?

> What decisions have we made on Project Apollo?

> What am I waiting for from other people?

And the assistant should be able to safely act:

> Add "send Sarah the architecture diagram" to my tasks for tomorrow.

> Create a meeting note for my conversation with Sarah about Project Apollo.

> Mark the Acme migration task complete.

> Add this decision to the Apollo project notes.

If those workflows are fast, reliable and pleasant, the fundamental product thesis has been proven.

---

# 36. The Product Boundary

The central architectural rule for the MVP should be:

> **Markdown owns the knowledge. The application owns the experience.**

Do not turn Markdown into a serialization format for an opaque database application.

Do not turn the application into a thin Markdown file browser either.

The application adds structure, relationships, workflows, task management, authentication, search, sharing, API access and automation while keeping the user's actual knowledge portable and understandable.

That tension is the product.

The other important boundary is:

> **The API is not an integration afterthought. It is a first-class interface to the user's knowledge system.**

Humans use the React interface.

Scripts use the HTTP API.

AI agents use MCP.

All three operate on the same underlying knowledge, relationships, tasks and permissions.

That is what can make this substantially more powerful than simply building another web-based Markdown editor.

The next useful step would be turning this into an **implementation spec**: repo structure, Go packages, SQLite schema, front-matter schemas, REST endpoints, MCP tools, auth flows, and a phased build order.
