package profiler

import (
	"path/filepath"
	"strings"

	"dataset-platform/backend/internal/filehandler"
)

func profileFile(file filehandler.AssembledFile) FileProfile {
	fp := FileProfile{
		FileID:       file.FileID,
		OriginalName: file.OriginalName,
		Extension:    strings.ToLower(filepath.Ext(file.OriginalName)),
		SizeBytes:    file.SizeBytes,
		MimeType:     file.MimeType,
		Role:         inferRole(file.OriginalName),
	}
	fp.DetectedType = detectType(fp.Extension, fp.MimeType, fp.OriginalName)

	switch fp.DetectedType {
	case "csv":
		profileDelimited(file.StoragePath, ',', &fp)
	case "tsv":
		profileDelimited(file.StoragePath, '\t', &fp)
	case "jsonl":
		profileJSONLines(file.StoragePath, &fp)
	case "json":
		profileJSON(file.StoragePath, &fp)
	case "xml":
		profileXML(file.StoragePath, &fp)
	case "text":
		profileText(file.StoragePath, &fp)
	case "parquet":
		profileParquet(file.StoragePath, &fp)
	case "hdf5":
		fp.Warnings = append(fp.Warnings, "HDF5 binary format: schema available via h5py/HDF5 tools; content not sampled by profiler")
	case "tfrecord":
		fp.Warnings = append(fp.Warnings, "TFRecord binary protobuf format: schema requires TensorFlow or tf.data to decode")
	case "arrow":
		fp.Warnings = append(fp.Warnings, "Apache Arrow/Feather binary format: schema available via pyarrow or pandas")
	case "webarchive":
		fp.Warnings = append(fp.Warnings, "Web archive format (WARC/WET/WAT): contains crawled web text; use warcio or resiliparse to process")
	}

	return fp
}
