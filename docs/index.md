---
name: Documentation Index
relation: README.md
description: Master table of contents with section-specific rules and folder structure overview
type: Enforce
---

# Documentation Index

## Structure

```
docs/
├── core/                   # System-wide ground truth (all Enforce)
│   ├── systemDesign.md     — Architecture and system design
│   ├── design.md           — Design decisions and rationale
│   ├── codingPrinciple.md  — Code standards and conventions
│   ├── testingRules.md     — Unit, component integration, and E2E API test rules
│   └── SKILLS.md           — Agent skills and subagent reference
└── modules/                # Domain patterns (Enforce rules + editable docs)
    ├── RULES.md            — Domain creation rules (Enforce)
    ├── docsRules.md        — Domain documentation boilerplate (Enforce)
    ├── featureTesting.md   — Procedure for testing a new feature (Enforce)
    ├── auth/               — Auth domain documentation
    ├── user/               — User domain documentation
    ├── berita/             — Berita (news) domain documentation
    └── ai/                 — AI (n8n webhook proxy) domain documentation
```

## Section-specific rules

### docs/core/

- All `type: Enforce`. Read these before any architectural or coding decisions.
- `systemDesign.md` takes precedence over all other docs for architecture questions.
- `design.md` records past decisions — do not contradict them without updating the file.
- `testingRules.md` governs all test code — read it before writing or modifying any test.

### docs/modules/

- `RULES.md` is `type: Enforce`. All domains must follow its rules.
- `docsRules.md` is `type: Enforce`. It is the boilerplate template for writing domain documentation. Use it whenever you document a new domain.
- `featureTesting.md` is `type: Enforce`. Follow its procedure whenever you implement or modify a feature that needs tests.
- Create new domain docs at the same level as `auth/`, not inside it.

## API Endpoints

All endpoints are prefixed with `/api/v1`.

| Method | Path | Description | Auth | Role |
|--------|------|-------------|------|------|
| `GET` | `/health` | Health check | No | — |
| `POST` | `/auth/register` | Register a new user account | No | — |
| `POST` | `/auth/login` | Login and receive JWT token | No | — |
| `POST` | `/auth/logout` | Logout and invalidate session | Yes | — |
| `GET` | `/users` | List all users | Yes | — |
| `GET` | `/users/{id}` | Get a user by ID | Yes | — |
| `POST` | `/users` | Create a new user | Yes | admin |
| `PUT` | `/users/{id}` | Update user by ID | Yes | admin |
| `PUT` | `/users/{id}/role` | Update user role | Yes | admin |
| `DELETE` | `/users/{id}` | Delete a user by ID | Yes | admin |
| `POST` | `/berita` | Create a new berita (news article) | Yes | jurnal |
| `GET` | `/berita` | List all berita | No | — |
| `GET` | `/berita/{id}` | Get a berita by ID | No | — |
| `PUT` | `/berita/{id}` | Update a berita by ID | Yes | jurnal |
| `DELETE` | `/berita/{id}` | Delete a berita by ID | Yes | jurnal |
| `POST` | `/berita/{id}/image` | Upload cover image for a berita | Yes | jurnal |
| `POST` | `/berita/{id}/images` | Upload content image for a berita | Yes | jurnal |
| `DELETE` | `/berita/{id}/images` | Delete a content image from a berita | Yes | jurnal |
| `POST` | `/approval/pkl` | Create a new PKL approval request | Yes | user |
| `GET` | `/approval/pkl` | List PKL approval requests | Yes | user, guru, admin |
| `GET` | `/approval/pkl/{id}` | Get a PKL approval request by ID | Yes | user, guru, admin |
| `POST` | `/approval/pkl/{id}/decide` | Approve/reject/cancel a PKL request | Yes | guru |
| `DELETE` | `/approval/pkl/{id}` | Cancel a PKL approval request | Yes | user |
| `POST` | `/nexxa/chat` | Send a chat message to Nexxa AI | No | — |
| `POST` | `/nexxa/match` | Run Nexxa matching (PPLG/akuntansi/hotel) | No | — |
| `POST` | `/nexxa/match/validate-input` | Validate Nexxa match input | No | — |
| `POST` | `/nexxa/match/normalize-output` | Normalize Nexxa match output | No | — |

### Role Legend

- **admin** — Administrator, full access
- **jurnal** — Journalist/writer, can create and manage berita
- **guru** — Teacher, can approve/reject PKL requests
- **user** — Regular user, can create and cancel PKL requests

### Middleware Notes

- **Auth** (`authMw`): Validates JWT token from `Authorization: Bearer <token>` header. Applied to all authenticated endpoints.
- **Role** (`roleMw`): Restricts access to specific roles. Applied after auth middleware on role-protected endpoints.
- **RateLimit**: Applied to Nexxa AI endpoints (`/nexxa/chat`, `/nexxa/match`) to prevent abuse.
