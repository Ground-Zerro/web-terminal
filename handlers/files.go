package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FileHandler handles file operations
type FileHandler struct {
	basePath string
}

// FileInfo represents a file or directory
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime string `json:"modTime"`
	Mode    string `json:"mode"`
}

// FilesResponse represents the response for file listing
type FilesResponse struct {
	Success bool       `json:"success"`
	Files   []FileInfo `json:"files"`
	Path    string     `json:"path"`
	Error   string     `json:"error,omitempty"`
}

// NewFileHandler creates a new file handler
func NewFileHandler() *FileHandler {
	return &FileHandler{
		basePath: "/",
	}
}

// ListFiles lists files in a directory
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get path from query
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	// Clean path
	path = filepath.Clean(path)

	// Security check: ensure path is within allowed directories
	if !strings.HasPrefix(path, h.basePath) {
		path = h.basePath
	}

	// Read directory
	entries, err := os.ReadDir(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, FilesResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to read directory: %v", err),
		})
		return
	}

	// Build file list
	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(path, entry.Name()),
			Size:    info.Size(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Mode:    info.Mode().String(),
		})
	}

	writeJSON(w, http.StatusOK, FilesResponse{
		Success: true,
		Files:   files,
		Path:    path,
	})
}

// UploadFile handles file upload
func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse form",
		})
		return
	}

	// Get target path
	targetPath := r.FormValue("path")
	if targetPath == "" {
		targetPath = "/"
	}

	// Get file from form
	file, handler, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to get file",
		})
		return
	}
	defer file.Close()

	// Create target path
	fullPath := filepath.Join(targetPath, handler.Filename)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create directory: %v", err),
		})
		return
	}

	// Create the file
	dst, err := os.Create(fullPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create file: %v", err),
		})
		return
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to save file: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"path":    fullPath,
	})
}

// DownloadFile handles file download
func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get file path
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing file path", http.StatusBadRequest)
		return
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Don't allow downloading directories
	if info.IsDir() {
		http.Error(w, "Cannot download directory", http.StatusBadRequest)
		return
	}

	// Set headers for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(filePath)))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	// Serve file
	http.ServeFile(w, r, filePath)
}

// CreateFolder creates a new directory
func (h *FileHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	// Validate
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Folder name is required",
		})
		return
	}

	// Clean path
	if req.Path == "" {
		req.Path = "/"
	}
	req.Path = filepath.Clean(req.Path)

	// Create full path
	fullPath := filepath.Join(req.Path, req.Name)

	// Check if already exists
	if _, err := os.Stat(fullPath); err == nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"success": false,
			"error":   "Folder already exists",
		})
		return
	}

	// Create directory
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create folder: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"path":    fullPath,
	})
}

// DeleteItem deletes a file or directory recursively
func (h *FileHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	// Validate
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Path is required",
		})
		return
	}

	// Clean path
	req.Path = filepath.Clean(req.Path)

	// Prevent deleting root
	if req.Path == "/" {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"success": false,
			"error":   "Cannot delete root directory",
		})
		return
	}

	// Check if exists
	info, err := os.Stat(req.Path)
	if os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"error":   "File or folder not found",
		})
		return
	}

	// Get name for response
	name := info.Name()

	// Delete file or directory
	if info.IsDir() {
		err = os.RemoveAll(req.Path)
	} else {
		err = os.Remove(req.Path)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to delete: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"name":    name,
	})
}

// DownloadFolder downloads a folder as a zip archive
func (h *FileHandler) DownloadFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get folder path
	folderPath := r.URL.Query().Get("path")
	if folderPath == "" {
		http.Error(w, "Missing folder path", http.StatusBadRequest)
		return
	}

	// Clean path
	folderPath = filepath.Clean(folderPath)

	// Check if folder exists
	info, err := os.Stat(folderPath)
	if os.IsNotExist(err) {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	if !info.IsDir() {
		http.Error(w, "Path is not a directory", http.StatusBadRequest)
		return
	}

	// Get folder name for zip filename
	folderName := filepath.Base(folderPath)
	zipName := folderName + ".zip"

	// Set headers for zip download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipName))

	// Create zip writer
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Walk through directory and add files
	err = filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create zip file header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// Use Store method (no compression) for fast speed
		header.Method = zip.Store

		// Set relative path in zip
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return err
		}

		// Use forward slashes for zip
		header.Name = filepath.ToSlash(relPath)
		if info.IsDir() {
			header.Name += "/"
		}

		// Create writer for this file
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// If it's a file (not directory), write contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			// Copy file contents
			_, err = io.Copy(writer, file)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create zip: %v", err), http.StatusInternalServerError)
		return
	}
}

// writeJSON writes JSON response (helper function)
func writeJSONFile(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
