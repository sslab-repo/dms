package filehandler

import "database/sql"

// AssembledFile describes one fully assembled file within a dataset.
// The AI pipeline receives a slice of these — one per file in the dataset.
type AssembledFile struct {
	FileID       int
	OriginalName string
	StoragePath  string
	SizeBytes    int64
	MimeType     string
}

// Handler manages all file I/O for the platform.
type Handler struct {
	storageDir string // root directory where completed files are stored
	tempDir    string // staging area for in-progress chunked uploads
	db         *sql.DB

	// OnFileAssembled is set by the router after construction.
	// It is called once ALL files belonging to a dataset have been
	// assembled — never once per individual file.
	// datasetID is the dataset these files belong to.
	// files is the complete list of assembled files for that dataset.
	OnFileAssembled func(datasetID int, files []AssembledFile)
}
