package models

type FileInfo struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime string
}
