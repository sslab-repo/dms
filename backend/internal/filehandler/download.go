package filehandler

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
)

// HandleDownload streams a completed dataset file to the client.
// It never loads the file into memory — io.Copy uses a 32 KB buffer internally,
// so even a 30 GB file streams without any memory pressure.
// Multi-file datasets are streamed as a zip archive.
func (h *Handler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	datasetIDStr := r.URL.Query().Get("dataset_id")
	datasetID, err := strconv.Atoi(datasetIDStr)
	if err != nil {
		http.Error(w, "invalid dataset_id", http.StatusBadRequest)
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, original_name, storage_path, size_bytes, coalesce(mime_type, '')
		   FROM files
		  WHERE dataset_id = $1 AND upload_status = 'complete'
		  ORDER BY id`,
		datasetID,
	)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type fileRow struct {
		id           int
		originalName string
		storagePath  string
		sizeBytes    int64
		mimeType     string
	}
	var files []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.id, &f.originalName, &f.storagePath, &f.sizeBytes, &f.mimeType); err != nil {
			http.Error(w, "database scan error", http.StatusInternalServerError)
			return
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if len(files) == 0 {
		http.Error(w, "no completed files found for dataset", http.StatusNotFound)
		return
	}

	if len(files) == 1 {
		f := files[0]
		file, err := os.Open(f.storagePath)
		if err != nil {
			http.Error(w, "file not found on disk", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		mimeType := f.mimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		safeName := sanitizeFilename(f.originalName)
		if safeName == "" {
			safeName = fmt.Sprintf("file-%d", f.id)
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(f.sizeBytes, 10))

		if _, err := io.Copy(w, file); err != nil {
			fmt.Printf("[Download] io.Copy failed for %s: %v\n", f.originalName, err)
		}
		return
	}

	for _, f := range files {
		if _, err := os.Stat(f.storagePath); err != nil {
			http.Error(w, "file not found on disk", http.StatusInternalServerError)
			return
		}
	}

	zipName := sanitizeFilename(fmt.Sprintf("dataset-%d.zip", datasetID))
	if zipName == "" {
		zipName = fmt.Sprintf("dataset-%d.zip", datasetID)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))
	w.Header().Set("Content-Type", "application/zip")

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	for _, f := range files {
		file, err := os.Open(f.storagePath)
		if err != nil {
			fmt.Printf("[Download] failed to open %s for ZIP: %v\n", f.storagePath, err)
			return
		}

		safeName := sanitizeFilename(f.originalName)
		if safeName == "" {
			safeName = fmt.Sprintf("file-%d", f.id)
		}
		header := &zip.FileHeader{
			Name:   safeName,
			Method: zip.Deflate,
		}
		if f.sizeBytes > 0 {
			header.UncompressedSize64 = uint64(f.sizeBytes)
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			fmt.Printf("[Download] failed to create ZIP entry for %s: %v\n", f.originalName, err)
			file.Close()
			return
		}

		if _, err := io.Copy(writer, file); err != nil {
			fmt.Printf("[Download] io.Copy failed for %s: %v\n", f.originalName, err)
			file.Close()
			return
		}
		file.Close()
	}
}
