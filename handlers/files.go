package handlers

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	uploadMemoryLimit = 32 << 20
	dirPerm           = 0o755
	modTimeLayout     = "2006-01-02 15:04:05"
)

var errOutsideBase = errors.New("path is outside the served root")

type FileHandler struct {
	basePath string
}

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime string `json:"modTime"`
	Mode    string `json:"mode"`
}

type filesResponse struct {
	Response
	Path  string     `json:"path"`
	Files []FileInfo `json:"files"`
}

type pathResponse struct {
	Response
	Path string `json:"path"`
}

type nameResponse struct {
	Response
	Name string `json:"name"`
}

func NewFileHandler(basePath string) *FileHandler {
	return &FileHandler{basePath: filepath.Clean(basePath)}
}

func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	only(http.MethodGet, h.list)(w, r)
}

func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	only(http.MethodPost, h.upload)(w, r)
}

func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	only(http.MethodGet, h.download)(w, r)
}

func (h *FileHandler) DownloadFolder(w http.ResponseWriter, r *http.Request) {
	only(http.MethodGet, h.downloadFolder)(w, r)
}

func (h *FileHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	only(http.MethodPost, h.mkdir)(w, r)
}

func (h *FileHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	only(http.MethodPost, h.delete)(w, r)
}

func (h *FileHandler) list(w http.ResponseWriter, r *http.Request) {
	path, _, err := h.resolveExisting(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, statusFor(err), "Failed to read directory: %v", err)
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		fail(w, statusFor(err), "Failed to read directory: %v", err)
		return
	}

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
			ModTime: info.ModTime().Format(modTimeLayout),
			Mode:    info.Mode().String(),
		})
	}

	writeJSON(w, http.StatusOK, filesResponse{
		Response: Response{Success: true},
		Path:     path,
		Files:    files,
	})
}

func (h *FileHandler) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(uploadMemoryLimit); err != nil {
		fail(w, http.StatusBadRequest, "Failed to parse form")
		return
	}
	defer r.MultipartForm.RemoveAll()

	dir, err := h.resolve(r.FormValue("path"))
	if err != nil {
		fail(w, statusFor(err), "Invalid target path: %v", err)
		return
	}

	src, header, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "Failed to get file")
		return
	}
	defer src.Close()

	name := filepath.Base(filepath.Clean(header.Filename))
	if name == "." || name == string(filepath.Separator) {
		fail(w, http.StatusBadRequest, "Invalid file name")
		return
	}

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		fail(w, statusFor(err), "Failed to create directory: %v", err)
		return
	}

	fullPath := filepath.Join(dir, name)
	dst, err := os.Create(fullPath)
	if err != nil {
		fail(w, statusFor(err), "Failed to create file: %v", err)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		fail(w, http.StatusInternalServerError, "Failed to save file: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, pathResponse{
		Response: Response{Success: true},
		Path:     fullPath,
	})
}

func (h *FileHandler) download(w http.ResponseWriter, r *http.Request) {
	path, info, err := h.resolveExisting(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	if !info.Mode().IsRegular() {
		http.Error(w, "Only regular files can be downloaded", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", attachment(filepath.Base(path)))
	http.ServeFile(w, r, path)
}

func (h *FileHandler) downloadFolder(w http.ResponseWriter, r *http.Request) {
	root, info, err := h.resolveExisting(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	if !info.IsDir() {
		http.Error(w, "Path is not a directory", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", attachment(filepath.Base(root)+".zip"))

	archive := zip.NewWriter(w)
	if err := writeZip(archive, root); err != nil {
		log.Printf("Failed to archive %s: %v", root, err)
		return
	}
	if err := archive.Close(); err != nil {
		log.Printf("Failed to finalise archive for %s: %v", root, err)
	}
}

func writeZip(archive *zip.Writer, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !entry.IsDir() {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Method = zip.Store
		header.Name = filepath.ToSlash(rel)
		if entry.IsDir() {
			header.Name += "/"
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

func (h *FileHandler) mkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "Invalid request")
		return
	}

	name := filepath.Base(filepath.Clean(req.Name))
	if req.Name == "" || name == "." || name == string(filepath.Separator) {
		fail(w, http.StatusBadRequest, "Folder name is required")
		return
	}

	parent, err := h.resolve(req.Path)
	if err != nil {
		fail(w, statusFor(err), "Invalid target path: %v", err)
		return
	}

	fullPath := filepath.Join(parent, name)
	if _, err := os.Stat(fullPath); err == nil {
		fail(w, http.StatusConflict, "Folder already exists")
		return
	}

	if err := os.MkdirAll(fullPath, dirPerm); err != nil {
		fail(w, statusFor(err), "Failed to create folder: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, pathResponse{
		Response: Response{Success: true},
		Path:     fullPath,
	})
}

func (h *FileHandler) delete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.Path == "" {
		fail(w, http.StatusBadRequest, "Path is required")
		return
	}

	path, info, err := h.resolveExisting(req.Path)
	if err != nil {
		fail(w, statusFor(err), "%v", err)
		return
	}
	if path == h.basePath {
		fail(w, http.StatusForbidden, "Cannot delete the root directory")
		return
	}

	if info.IsDir() {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		fail(w, statusFor(err), "Failed to delete: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, nameResponse{
		Response: Response{Success: true},
		Name:     info.Name(),
	})
}

func (h *FileHandler) resolve(raw string) (string, error) {
	if raw == "" {
		return h.basePath, nil
	}

	path := filepath.Clean(raw)
	if !filepath.IsAbs(path) {
		return "", errOutsideBase
	}

	rel, err := filepath.Rel(h.basePath, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errOutsideBase
	}
	return path, nil
}

func (h *FileHandler) resolveExisting(raw string) (string, os.FileInfo, error) {
	path, err := h.resolve(raw)
	if err != nil {
		return "", nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	return path, info, nil
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, errOutsideBase):
		return http.StatusForbidden
	case errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, fs.ErrPermission):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func attachment(name string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}
