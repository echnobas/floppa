package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	_ "modernc.org/sqlite"
)

type Files struct {
	s3 *minio.Client
	db *sql.DB
}

func NewFiles() Files {
	endpoint := "storage:9000"
	token := "minioadmin"
	secret := "minioadmin"
	_s3, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(token, secret, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalln(err)
	}

	if exists, err := _s3.BucketExists(context.Background(), "files"); !(exists && err != nil) {
		_s3.MakeBucket(context.Background(), "files", minio.MakeBucketOptions{})
	}

	sqlite3, err := sql.Open("sqlite", "files.db")
	if err != nil {
		log.Fatalln(err)
	}

	_, err = sqlite3.Exec(
		`CREATE TABLE IF NOT EXISTS files (
	guid TEXT PRIMARY KEY,
	filename TEXT NOT NULL,
	timestamp DEFAULT (datetime('now'))
);`)
	if err != nil {
		log.Fatalln(err)
	}

	files := Files{
		s3: _s3,
		db: sqlite3,
	}

	return files
}

func (f *Files) Download(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	guid, ok := vars["guid"]
	if !ok {
		http.Error(w, `{"error": "guid required"}`, http.StatusBadRequest)
	}

	var filename string
	row := f.db.QueryRow("SELECT filename FROM files WHERE guid = ?", guid)
	err := row.Scan(&filename)
	switch err {
	case sql.ErrNoRows:
		fmt.Printf("db[load] :: %s/%s FAILED!\n", guid, filename)
		http.Error(w, `{"error": "file not found"}`, http.StatusNotFound)
		return
	case nil:
		object, err := f.s3.GetObject(r.Context(), "files", guid+"/"+filename, minio.GetObjectOptions{})
		defer object.Close()
		if err != nil {
			fmt.Printf("s3[load] :: %s/%s FAILED!\n", guid, filename)
			http.Error(w, `{"error": "internal"}`, http.StatusInternalServerError)
			return
		}
		stat, err := object.Stat()
		if err != nil {
			fmt.Printf("s3[stat] :: %s/%s FAILED!\n", guid, filename)
			http.Error(w, `{"error": "internal"}`, http.StatusInternalServerError)
			return
		}
		fmt.Printf("s3[load] :: %s/%s\n", guid, filename)
		w.Header().Add("content-length", strconv.FormatInt(stat.Size, 10))
		w.Header().Add("content-type", "application/octet-stream")
		w.Header().Set("content-disposition", `attachment; filename="`+filename+`"`)
		w.WriteHeader(http.StatusOK)
		io.Copy(w, object)
		return
	}
	http.Error(w, `{"error": "internal"}`, http.StatusInternalServerError)
}

func (f *Files) Upload(w http.ResponseWriter, r *http.Request) {
	const (
		MAX_CONTENT_LENTH = 6368709120
	)

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	filename, ok := vars["filename"]
	if !ok {
		http.Error(w, `{"error": "filename required"}`, http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_CONTENT_LENTH)
	if r.ContentLength == -1 {
		http.Error(w, `{"error": "content-length not present"}`, http.StatusBadRequest)
		return
	}

	if r.ContentLength > MAX_CONTENT_LENTH {
		http.Error(w, `{"error": "maximum filesize exceeded"}`, http.StatusBadRequest)
		return
	}

	guid := uuid.New().String()
	_, err := f.s3.PutObject(r.Context(), "files", guid+"/"+filename, r.Body, -1, minio.PutObjectOptions{})
	if err != nil {
		fmt.Println(err)
		fmt.Printf("s3[save] :: %s/%s FAILED!\n", guid, filename)
		http.Error(w, `{"error": "internal"}`, http.StatusInternalServerError)
		return
	}
	fmt.Printf("s3[save] :: %s/%s\n", guid, filename)

	_, err = f.db.Exec("INSERT INTO files(guid, filename) VALUES(?, ?)", guid, filename)
	if err != nil {
		fmt.Printf("db[register] :: %s->%s FAILED!\n", guid, filename)
		f.s3.RemoveObject(r.Context(), "files", guid+"/"+filename, minio.RemoveObjectOptions{})
		fmt.Printf("s3[delete] :: %s/%s\n", guid, filename)
		http.Error(w, `{"error": "internal"}`, http.StatusInternalServerError)
		return
	}
	fmt.Printf("db[register] :: %s->%s\n", guid, filename)
	w.Write([]byte(fmt.Sprintf(`{"guid": "%s"}`, guid)))
}
