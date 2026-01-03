package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// URLMapping stores the mapping between short code and original URL
type URLMapping struct {
	ShortCode   string
	OriginalURL string
}

// Shortener holds the application state
type Shortener struct {
	// URLs stores the mappings: short code -> original URL
	URLs map[string]string
	// Reverse mapping for checking duplicates (optional)
	ReverseURLs map[string]string
	// Mutex to protect concurrent access to the maps
	mu sync.RWMutex
	// HTML template
	tmpl *template.Template
}

// NewShortener creates a new Shortener instance
func NewShortener() *Shortener {
	// Parse the HTML template
	tmpl := template.Must(template.ParseFiles("templates/index.html"))

	return &Shortener{
		URLs:        make(map[string]string),
		ReverseURLs: make(map[string]string),
		tmpl:        tmpl,
	}
}

// generateShortCode creates a random 6-character string for the short URL
// Uses base64 URL encoding for safety in URLs
func (s *Shortener) generateShortCode() (string, error) {
	// Generate 6 random bytes
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode to base64 URL format and take first 6 characters
	// Base64 URL encoding uses A-Z, a-z, 0-9, - and _ (URL-safe)
	encoded := base64.URLEncoding.EncodeToString(bytes)

	// Take the first 6 characters and ensure they're URL-safe
	// Replace any + or / that might appear (though URLEncoding should prevent this)
	code := strings.ReplaceAll(encoded[:6], "+", "-")
	code = strings.ReplaceAll(code, "/", "_")

	return code, nil
}

// shortenURL creates a short code for the given URL
func (s *Shortener) shortenURL(originalURL string) (string, error) {
	// Parse the URL to validate it
	parsed, err := url.Parse(originalURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %v", err)
	}

	// Ensure the URL has a scheme (http or https)
	if parsed.Scheme == "" {
		originalURL = "http://" + originalURL
		// Re-parse to validate the new URL
		if _, err := url.Parse(originalURL); err != nil {
			return "", fmt.Errorf("invalid URL: %v", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we already have this URL (optional optimization)
	// This prevents creating multiple short codes for the same URL
	if code, exists := s.ReverseURLs[originalURL]; exists {
		return code, nil
	}

	// Generate a unique short code
	var code string
	for {
		// Keep generating until we get a unique code
		// In practice with 6 chars (62^6 possibilities), collisions are very rare
		newCode, err := s.generateShortCode()
		if err != nil {
			return "", fmt.Errorf("failed to generate short code: %v", err)
		}

		// Check if this code is already in use
		if _, exists := s.URLs[newCode]; !exists {
			code = newCode
			break
		}
		// If code exists, loop and try again (extremely rare)
	}

	// Store the mapping
	s.URLs[code] = originalURL
	s.ReverseURLs[originalURL] = code

	return code, nil
}

// handleIndex renders the main page with the form
func (s *Shortener) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Only handle GET requests for the index
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Don't serve anything at paths other than root
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Prepare data for the template
	data := struct {
		ShortURL string
		Error    string
		URLs     []URLMapping
	}{}

	// Get recent URLs for display (last 10)
	s.mu.RLock()
	count := 0
	for code, originalURL := range s.URLs {
		if count >= 10 { // Show only last 10
			break
		}
		data.URLs = append(data.URLs, URLMapping{
			ShortCode:   code,
			OriginalURL: originalURL,
		})
		count++
	}
	s.mu.RUnlock()

	// Execute the template
	if err := s.tmpl.Execute(w, data); err != nil {
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}

// handleShorten processes the form submission to create a short URL
func (s *Shortener) handleShorten(w http.ResponseWriter, r *http.Request) {
	// Only handle POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Get the URL from the form
	originalURL := strings.TrimSpace(r.FormValue("url"))
	if originalURL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Create short code
	code, err := s.shortenURL(originalURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build the short URL
	shortURL := fmt.Sprintf("http://%s/%s", r.Host, code)

	// Prepare data for the template
	data := struct {
		ShortURL string
		Error    string
		URLs     []URLMapping
	}{
		ShortURL: shortURL,
	}

	// Get recent URLs for display
	s.mu.RLock()
	count := 0
	for code, origURL := range s.URLs {
		if count >= 10 {
			break
		}
		data.URLs = append(data.URLs, URLMapping{
			ShortCode:   code,
			OriginalURL: origURL,
		})
		count++
	}
	s.mu.RUnlock()

	// Execute the template with the result
	if err := s.tmpl.Execute(w, data); err != nil {
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}

// handleRedirect redirects from short code to original URL
func (s *Shortener) handleRedirect(w http.ResponseWriter, r *http.Request) {
	// Only handle GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract the short code from the URL path
	// The path will be like "/abc123"
	code := strings.TrimPrefix(r.URL.Path, "/")

	// Look up the original URL
	s.mu.RLock()
	originalURL, exists := s.URLs[code]
	s.mu.RUnlock()

	if !exists {
		http.NotFound(w, r)
		return
	}

	// Redirect to the original URL
	// Use StatusFound (302) for temporary redirect
	http.Redirect(w, r, originalURL, http.StatusFound)
}

// StartServer initializes and starts the HTTP server
func (s *Shortener) StartServer(addr string) error {
	// Create a new HTTP request multiplexer
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Route based on the path and method
		if r.URL.Path == "/" {
			s.handleIndex(w, r)
		} else if r.URL.Path == "/shorten" {
			s.handleShorten(w, r)
		} else {
			s.handleRedirect(w, r)
		}
	})

	// Log server start
	log.Printf("Starting server on %s", addr)
	log.Printf("Visit http://%s to use the URL shortener", addr)

	// Start the server
	return http.ListenAndServe(addr, mux)
}

func main() {
	// Create a new shortener instance
	shortener := NewShortener()

	// Start the server on port 8080
	// You can change the port by setting the PORT environment variable
	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	// Start the server
	if err := shortener.StartServer(addr); err != nil {
		log.Fatal("Server failed:", err)
	}
}
