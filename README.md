# Leave Compliance

Automated attendance sync and leave compliance checker. Reads biometric attendance data from an on-premise SQL Server (eSSL/eTimeTrack), identifies absentees, cross-references with GreytHR leave records, and reports non-compliant employees.

## How It Works

```
eSSL Biometric Device
        │
        ▼
eTimeTrack → SQL Server (on-premise)
        │
        ▼
┌───────────────────────────┐
│  Attendance Sync (Go CLI) │  ← this repo
└───────────────────────────┘
        │
        ▼
Cloud API / Leave Check Pipeline
        │
        ▼
Absentee Report + Email Notifications
```

## Components

| Component | Path | Description |
|-----------|------|-------------|
| Attendance Sync | `main.go` | Fetches punch logs from SQL Server, groups by date, posts to cloud API |
| Leave Check | `scripts/leaveCheck/` | Compares present list against active employees and GreytHR leave data |
| Send Mail | `scripts/sendMail/` | Sends absentee notification emails |
| Store/Get Present List | `scripts/storePresentListInTable/`, `scripts/getPresentListFromTable/` | Persists daily attendance to a table store |
| Get All Employees | `scripts/getAllEmployee/` | Fetches active employee list from GreytHR |
| Token Helpers | `scripts/getGreytHRToken/`, `scripts/getMicrosoftToken/` | OAuth token acquisition |

## Setup

### Prerequisites

- Go 1.21+
- Access to the on-premise SQL Server (eSSL database)

### Environment Variables

Create a `.env` file:

```env
DB_HOST=192.168.x.x
DB_PORT=1433
DB_NAME=your-db-name
DB_USER=your-db-username
DB_PASSWORD=***

DAYS_TO_FETCH=7

# Optional — omit for dry-run mode
API_URL=https://your-cloud-endpoint/api/attendance
API_KEY=your-api-key
API_KEY_HEADER=X-Api-Key
```

### Build & Run

```bash
# Run directly
go run .

# Build for Windows (target machine)
make build-windows

# Tidy dependencies
make tidy
```

When `API_URL` is not set, the tool runs in **dry-run mode** — it fetches and processes data but does not POST anywhere.

## Project Structure

```
├── main.go          # Entry point: fetch → process → post
├── config.go        # Environment variable loading
├── db.go            # SQL Server connection and attendance query
├── process.go       # Groups raw punches into per-day attendance
├── api.go           # HTTP POST to cloud API
├── makefile         # Build targets
├── scripts/
│   ├── leaveCheck/         # Absentee detection + leave cross-check
│   ├── sendMail/           # Email notifications
│   ├── getAllEmployee/     # GreytHR employee fetch
│   ├── getGreytHRToken/    # GreytHR OAuth
│   ├── getMicrosoftToken/  # Microsoft OAuth
│   ├── storePresentListInTable/
│   └── getPresentListFromTable/
└── docs/            # Architecture notes and design specs
```

## Testing

```bash
go test ./...
```
