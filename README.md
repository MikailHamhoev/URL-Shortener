# Simple URL Shortener

A lightweight, in-memory URL shortening service built with Go's standard library.

## Features
- Creates short URLs from long ones (e.g., `http://localhost:8080/abc123`)
- In-memory storage with thread-safe access
- Automatic HTTP scheme detection (`http://` added if missing)
- Duplicate prevention: same URL always returns the same short code
- Web interface with form input and recent URLs list
- No external dependencies — pure Go standard library

## Quick Start
```bash
go run main.go
```
Visit http://localhost:8080 to use the service.

## Usage
1. Enter a full URL (e.g., `https://example.com`) in the input field  
2. Click **Shorten**  
3. Copy the generated short URL (e.g., `http://localhost:8080/xyz789`)  
4. Share it — visiting the short URL redirects to the original

## Configuration
Set the `PORT` environment variable to change the listening port:
```bash
PORT=3000 go run main.go
```

## Project Structure
```
.
├── main.go              # Core logic and HTTP server
└── templates/
    └── index.html       # Web UI template
```

## Notes
- Data is stored only in memory — all URLs are lost when the server stops  
- Short codes are 6-character, URL-safe strings generated using cryptographically secure randomness  
- Supports up to ~56 billion unique combinations (62⁶)

MIT License