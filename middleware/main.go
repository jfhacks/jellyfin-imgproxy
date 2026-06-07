package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func main() {
	const dbPath = "/var/lib/jellyfin/data/jellyfin.db"
	dsn := "file:" + dbPath + "?mode=ro&_busy_timeout=5000"
	var err error
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)

	http.HandleFunc("/resolve", resolveHandler)
	srv := &http.Server{
		Addr:         "127.0.0.1:18887",
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s, db=%s", srv.Addr, dbPath)
	log.Fatal(srv.ListenAndServe())
}

func resolveHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	jfID := hyphenateGUID(q.Get("jf_id"))
	imgID := q.Get("img_id")
	imgType := strings.ToLower(q.Get("img_type"))
	size := q.Get("size")

	// log.Printf("REQ %q %q", jfID, q) // Log request for debugging

	var raw string
	var err error

	// screenshot & profile missing - never found option in Jellyfin for them
	var ImageTypeMap = map[string]string{
		"default":  "0",
		"primary":  "0",
		"art":      "1",
		"backdrop": "2",
		"banner":   "3",
		"logo":     "4",
		"thumb":    "5",
		"disc":     "6",
		"box":      "7",
		"menu":     "9",
		"back":     "11",
	}

	if imgType == "" {
		http.Error(w, "image requires img_type", http.StatusBadRequest)
		return
	}

	switch imgType {
	case "backdrop":
		raw, err = queryBackdrop(jfID, ImageTypeMap[imgType], imgID)
	case "chapter":
		raw, err = queryChapter(jfID, imgID)
	default:
		raw, err = queryBaseItemImageInfo(jfID, ImageTypeMap[imgType])
	}

	if err == sql.ErrNoRows || raw == "" {
		log.Printf("SQLite file pointer inaccessible - restart service")
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("db error: %v", err)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}

	final := normalize(raw)

	streamImgFromImgproxy(w, final, size)
}

func streamImgFromImgproxy(w http.ResponseWriter, final string, size string) {
	enc := url.PathEscape(final)

	imgURL := fmt.Sprintf(
		"http://127.0.0.1:18889/insecure/%s/plain/local://%s@webp",
		size,
		enc,
	)

	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		http.Error(w, "bad upstream request", http.StatusInternalServerError)
		return
	}
	// minimal upstream headers
	req.Header.Set("Accept", "image/*")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// forward only Content-Type and Content-Length
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func queryChapter(itemId, chapterIndex string) (string, error) {
	var p sql.NullString
	err := db.QueryRow(`SELECT ImagePath FROM Chapters WHERE ItemId LIKE ? AND ChapterIndex = ? LIMIT 1`, itemId, chapterIndex).Scan(&p)
	if err != nil {
		return "", err
	}

	return p.String, nil
}

func queryBackdrop(itemId, imageType, offsetStr string) (string, error) {
	var p sql.NullString
	err := db.QueryRow(`SELECT Path FROM BaseItemImageInfos WHERE ItemId LIKE ? AND ImageType = ? ORDER BY Path ASC LIMIT 1 OFFSET ?`,
		itemId, imageType, offsetStr).Scan(&p)
	if err != nil {
		return "", err
	}

	return p.String, nil
}

func queryBaseItemImageInfo(itemId, imageType string) (string, error) {
	var p sql.NullString
	err := db.QueryRow(`SELECT Path FROM BaseItemImageInfos WHERE ItemId LIKE ? AND ImageType = ? LIMIT 1`, itemId, imageType).Scan(&p)
	if err != nil {
		return "", err
	}

	return p.String, nil
}

func normalize(p string) string {
	if p == "" {
		return ""
	}
	const meta = "/var/lib/jellyfin/metadata"
	const coll = "/var/lib/jellyfin/data"

	if strings.HasPrefix(p, coll) {
		return strings.TrimPrefix(p, coll)
	}
	if strings.HasPrefix(p, meta) {
		return "/metadata" + strings.TrimPrefix(p, meta)
	}

	return p
}

// different apps use different uuid formats
func hyphenateGUID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) == 36 {
		return strings.ToLower(id)
	}

	if len(id) == 32 {
		// 8-4-4-4-12
		return strings.ToLower(id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:32])
	}

	return id
}
