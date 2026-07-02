// Main Application
class App {
    constructor() {
        this.token = null;
        this.terminalManager = null;
        this.currentPath = '/';
        this.sortField = this.getCookie('sortField') || 'name';
        this.sortAsc = this.getCookie('sortAsc') !== 'false';
        this.files = [];

        // Initialize
        this.init();
    }

    init() {
        // Check for existing session
        this.token = sessionStorage.getItem('token');
        if (this.token) {
            this.showApp();
        } else {
            this.showLogin();
        }

        // Setup event listeners
        this.setupEventListeners();
    }

    setupEventListeners() {
        // Login form
        document.getElementById('login-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.login();
        });

        // Logout button
        document.getElementById('logout-btn').addEventListener('click', () => {
            this.logout();
        });

        // File browser
        document.getElementById('refresh-files').addEventListener('click', () => {
            this.loadFiles(this.currentPath);
        });

        document.getElementById('new-folder').addEventListener('click', () => {
            this.createNewFolder();
        });

        // Sort buttons
        document.getElementById('sort-name').addEventListener('click', () => {
            this.toggleSort('name');
        });
        document.getElementById('sort-size').addEventListener('click', () => {
            this.toggleSort('size');
        });
        document.getElementById('sort-date').addEventListener('click', () => {
            this.toggleSort('date');
        });

        // Path input — Enter to navigate, typing triggers autocomplete
        const pathInput = document.getElementById('current-path');
        const pathAutocomplete = document.getElementById('path-autocomplete');
        let autocompleteTimeout = null;
        let selectedSuggestion = -1;

        pathInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                this.loadFiles(pathInput.value || '/');
                pathAutocomplete.classList.remove('active');
                return;
            }
            if (e.key === 'Escape') {
                pathAutocomplete.classList.remove('active');
                return;
            }
            // Arrow navigation in autocomplete
            const items = pathAutocomplete.querySelectorAll('.path-suggestion');
            if (items.length > 0) {
                if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    selectedSuggestion = Math.min(selectedSuggestion + 1, items.length - 1);
                    items.forEach((el, i) => el.classList.toggle('selected', i === selectedSuggestion));
                    return;
                }
                if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    selectedSuggestion = Math.max(selectedSuggestion - 1, 0);
                    items.forEach((el, i) => el.classList.toggle('selected', i === selectedSuggestion));
                    return;
                }
                if (e.key === 'Tab' && selectedSuggestion >= 0) {
                    e.preventDefault();
                    pathInput.value = items[selectedSuggestion].dataset.path;
                    pathAutocomplete.classList.remove('active');
                    return;
                }
            }
        });

        pathInput.addEventListener('input', () => {
            clearTimeout(autocompleteTimeout);
            selectedSuggestion = -1;
            const val = pathInput.value.trim();
            if (!val) {
                pathAutocomplete.classList.remove('active');
                return;
            }
            autocompleteTimeout = setTimeout(() => this.suggestPath(val), 200);
        });

        pathInput.addEventListener('focus', () => {
            if (pathAutocomplete.children.length > 0 && pathInput.value.trim()) {
                pathAutocomplete.classList.add('active');
            }
        });

        // Click outside to close autocomplete
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.file-browser-path')) {
                pathAutocomplete.classList.remove('active');
            }
        });

        // File upload
        const fileInput = document.getElementById('file-input');
        const uploadArea = document.getElementById('upload-area');

        fileInput.addEventListener('change', (e) => {
            this.uploadFiles(e.target.files);
        });

        // Drag and drop -支持文件和文件夹
        uploadArea.addEventListener('dragover', (e) => {
            e.preventDefault();
            e.stopPropagation();
            uploadArea.classList.add('dragover');
        });

        uploadArea.addEventListener('dragleave', (e) => {
            e.preventDefault();
            e.stopPropagation();
            uploadArea.classList.remove('dragover');
        });

        uploadArea.addEventListener('drop', (e) => {
            e.preventDefault();
            e.stopPropagation();
            uploadArea.classList.remove('dragover');

            const items = e.dataTransfer.items;
            if (items) {
                this.uploadItems(items);
            } else {
                this.uploadFiles(e.dataTransfer.files);
            }
        });

        // Terminal clear button
        document.getElementById('clear-terminal').addEventListener('click', () => {
            if (this.terminalManager) {
                this.terminalManager.clear();
            }
        });

        // Initialize sort UI
        this.updateSortUI();
    }

    showLogin() {
        document.getElementById('login-screen').style.display = 'flex';
        document.getElementById('app').style.display = 'none';
    }

    showApp() {
        document.getElementById('login-screen').style.display = 'none';
        document.getElementById('app').style.display = 'flex';

        // Initialize terminal
        this.terminalManager = new TerminalManager();
        this.terminalManager.init(this.token);

        // Start performance widget
        window.perfWidget.start(this.token);

        // Load files
        this.loadFiles('/root/');
    }

    async login() {
        const login = document.getElementById('login').value;
        const password = document.getElementById('password').value;
        const errorDiv = document.getElementById('login-error');

        try {
            const response = await fetch('api/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ login, password })
            });

            const data = await response.json();

            if (data.success) {
                this.token = data.token;
                sessionStorage.setItem('token', this.token);
                this.showApp();
            } else {
                errorDiv.textContent = data.error || 'Login failed';
                errorDiv.style.display = 'block';
            }
        } catch (error) {
            errorDiv.textContent = 'Connection error';
            errorDiv.style.display = 'block';
        }
    }

    async logout() {
        try {
            await fetch('api/logout', {
                method: 'POST',
                headers: {
                    'Authorization': this.token,
                }
            });
        } catch (error) {
            console.error('Logout error:', error);
        }

        // Stop performance widget
        window.perfWidget.stop();

        // Disconnect terminal
        if (this.terminalManager) {
            this.terminalManager.disconnect();
        }

        // Clear token
        this.token = null;
        sessionStorage.removeItem('token');

        // Show login
        this.showLogin();
    }

    async loadFiles(path) {
        try {
            const response = await fetch(`api/files?path=${encodeURIComponent(path)}`, {
                headers: {
                    'Authorization': this.token,
                }
            });

            const data = await response.json();

            if (data.success) {
                this.currentPath = data.path;
                document.getElementById('current-path').value = data.path;
                this.files = data.files;
                this.renderFiles();
            }
        } catch (error) {
            console.error('Load files error:', error);
        }
    }

    async suggestPath(input) {
        const autocomplete = document.getElementById('path-autocomplete');
        autocomplete.innerHTML = '';

        // Determine parent directory and prefix to match
        let parentDir, prefix;
        if (input.endsWith('/')) {
            parentDir = input.slice(0, -1) || '/';
            prefix = '';
        } else {
            const parts = input.split('/');
            prefix = parts.pop();
            parentDir = parts.join('/') || '/';
        }

        try {
            const response = await fetch(`api/files?path=${encodeURIComponent(parentDir)}`, {
                headers: { 'Authorization': this.token }
            });
            const data = await response.json();
            if (!data.success) return;

            const matches = data.files
                .filter(f => f.isDir && f.name.toLowerCase().startsWith(prefix.toLowerCase()))
                .map(f => {
                    const fullPath = parentDir === '/' ? `/${f.name}` : `${parentDir}/${f.name}`;
                    return { name: f.name, path: fullPath + '/' };
                })
                .sort((a, b) => a.name.localeCompare(b.name));

            if (matches.length === 0) {
                autocomplete.classList.remove('active');
                return;
            }

            matches.forEach(m => {
                const div = document.createElement('div');
                div.className = 'path-suggestion';
                div.textContent = m.path;
                div.dataset.path = m.path;
                div.addEventListener('click', () => {
                    document.getElementById('current-path').value = m.path;
                    autocomplete.classList.remove('active');
                    this.loadFiles(m.path);
                });
                autocomplete.appendChild(div);
            });

            autocomplete.classList.add('active');
        } catch (e) {
            // silent
        }
    }

    // Sort functions
    toggleSort(field) {
        if (this.sortField === field) {
            this.sortAsc = !this.sortAsc;
        } else {
            this.sortField = field;
            this.sortAsc = true;
        }

        // Save to cookies
        this.setCookie('sortField', this.sortField, 365);
        this.setCookie('sortAsc', this.sortAsc.toString(), 365);

        this.updateSortUI();
        this.renderFiles();
    }

    updateSortUI() {
        // Reset all sort buttons
        document.querySelectorAll('.sort-btn').forEach(btn => {
            btn.classList.remove('active', 'asc', 'desc');
        });

        // Set active sort button
        const activeBtn = document.getElementById(`sort-${this.sortField}`);
        if (activeBtn) {
            activeBtn.classList.add('active');
            activeBtn.classList.add(this.sortAsc ? 'asc' : 'desc');
        }
    }

    sortFiles(files) {
        const sorted = [...files];

        // Separate directories and files
        const dirs = sorted.filter(f => f.isDir);
        const fileItems = sorted.filter(f => !f.isDir);

        // Sort function
        const compare = (a, b) => {
            let result = 0;

            switch (this.sortField) {
                case 'name':
                    result = a.name.localeCompare(b.name);
                    break;
                case 'size':
                    result = (a.size || 0) - (b.size || 0);
                    break;
                case 'date':
                    result = new Date(a.modTime || 0) - new Date(b.modTime || 0);
                    break;
            }

            return this.sortAsc ? result : -result;
        };

        // Sort directories and files separately
        dirs.sort(compare);
        fileItems.sort(compare);

        // Return dirs first, then files
        return [...dirs, ...fileItems];
    }

    renderFiles() {
        const fileList = document.getElementById('file-list');
        fileList.innerHTML = '';

        // Add parent directory link
        if (this.currentPath !== '/') {
            const parentPath = this.currentPath.split('/').slice(0, -1).join('/') || '/';
            const parentItem = this.createFileItem({
                name: '..',
                path: parentPath,
                isDir: true
            });
            fileList.appendChild(parentItem);
        }

        // Sort and add files
        const sortedFiles = this.sortFiles(this.files);
        sortedFiles.forEach(file => {
            const fileItem = this.createFileItem(file);
            fileList.appendChild(fileItem);
        });
    }

    createFileItem(file) {
        const div = document.createElement('div');
        div.className = 'file-item';

        const icon = file.isDir ? '📁' : this.getFileIcon(file.name);
        const size = file.isDir ? '' : this.formatSize(file.size);

        div.innerHTML = `
            <span class="file-icon">${icon}</span>
            <div class="file-info">
                <div class="file-name" title="${file.name}">${file.name}</div>
                <div class="file-meta">${size} ${file.modTime || ''}</div>
            </div>
            <div class="file-actions">
                ${file.isDir
                    ? `<button class="btn-download" onclick="app.downloadFolder('${file.path}')" title="Download as ZIP">📦</button>`
                    : `<button class="btn-download" onclick="app.downloadFile('${file.path}')" title="Download">⬇</button>`
                }
                <button class="btn-delete" onclick="app.deleteItem('${file.path}', '${file.name}', ${file.isDir})">🗑️</button>
            </div>
        `;

        // Click to navigate
        div.addEventListener('click', (e) => {
            if (e.target.classList.contains('btn-download') || e.target.classList.contains('btn-delete')) {
                return;
            }
            if (file.isDir) {
                this.loadFiles(file.path);
            }
        });

        // Middle click — paste path into terminal
        div.addEventListener('mousedown', (e) => {
            if (e.button === 1) {
                e.preventDefault();
                if (this.terminalManager) {
                    this.terminalManager.sendText(file.path + (file.isDir ? '/' : ''));
                }
            }
        });

        return div;
    }

    getFileIcon(filename) {
        const ext = filename.split('.').pop().toLowerCase();
        const icons = {
            'js': '📜',
            'ts': '📜',
            'py': '🐍',
            'go': '🔵',
            'rs': '🦀',
            'java': '☕',
            'c': '⚙️',
            'cpp': '⚙️',
            'h': '📄',
            'html': '🌐',
            'css': '🎨',
            'json': '📋',
            'xml': '📋',
            'yaml': '📋',
            'yml': '📋',
            'md': '📝',
            'txt': '📝',
            'log': '📋',
            'sh': '🐚',
            'bash': '🐚',
            'zip': '📦',
            'tar': '📦',
            'gz': '📦',
            'jpg': '🖼️',
            'jpeg': '🖼️',
            'png': '🖼️',
            'gif': '🖼️',
            'svg': '🖼️',
            'mp3': '🎵',
            'mp4': '🎬',
            'pdf': '📄',
            'doc': '📄',
            'docx': '📄',
            'xls': '📊',
            'xlsx': '📊'
        };
        return icons[ext] || '📄';
    }

    formatSize(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    // Upload items (supports both files and folders)
    async uploadItems(items) {
        const entries = [];
        const queue = [];

        // Process items
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            if (item.kind === 'file') {
                const entry = item.webkitGetAsEntry ? item.webkitGetAsEntry() : null;
                if (entry) {
                    queue.push({ entry, path: this.currentPath });
                } else {
                    // Fallback for browsers withoutwebkitGetAsEntry
                    const file = item.getAsFile();
                    if (file) {
                        entries.push({ file, path: this.currentPath });
                    }
                }
            }
        }

        // Process directory entries recursively
        while (queue.length > 0) {
            const { entry, path } = queue.shift();

            if (entry.isFile) {
                const file = await new Promise((resolve) => entry.file(resolve));
                entries.push({ file, path });
            } else if (entry.isDirectory) {
                const dirReader = entry.createReader();
                const dirEntries = await new Promise((resolve) => {
                    const allEntries = [];
                    const readEntries = () => {
                        dirReader.readEntries((ents) => {
                            if (ents.length === 0) {
                                resolve(allEntries);
                            } else {
                                allEntries.push(...ents);
                                readEntries();
                            }
                        });
                    };
                    readEntries();
                });

                // Add subdirectory to queue
                const newPath = path === '/' ? `/${entry.name}` : `${path}/${entry.name}`;
                for (const subEntry of dirEntries) {
                    queue.push({ entry: subEntry, path: newPath });
                }
            }
        }

        // Upload all files with progress
        if (entries.length > 0) {
            this.showProgress(`${entries.length} file(s)`);
            for (let i = 0; i < entries.length; i++) {
                const { file, path } = entries[i];
                document.getElementById('progress-text').textContent =
                    `Uploading ${i + 1}/${entries.length}: ${file.name}`;
                await this.uploadFile(file, path);
            }
            this.hideProgress();
        }

        // Refresh file list
        this.loadFiles(this.currentPath);
    }

    async uploadFiles(files) {
        if (files.length > 0) {
            this.showProgress(`${files.length} file(s)`);
            for (let i = 0; i < files.length; i++) {
                document.getElementById('progress-text').textContent =
                    `Uploading ${i + 1}/${files.length}: ${files[i].name}`;
                await this.uploadFile(files[i], this.currentPath);
            }
            this.hideProgress();
        }
        this.loadFiles(this.currentPath);
    }

    async uploadFile(file, path) {
        return new Promise((resolve) => {
            const formData = new FormData();
            formData.append('file', file);
            formData.append('path', path);

            const xhr = new XMLHttpRequest();
            xhr.open('POST', 'api/files/upload');
            xhr.setRequestHeader('Authorization', this.token);

            xhr.upload.onprogress = (e) => {
                if (e.lengthComputable) {
                    const pct = Math.round((e.loaded / e.total) * 100);
                    this.updateProgress(pct, file.name);
                }
            };

            xhr.onload = () => {
                try {
                    const data = JSON.parse(xhr.responseText);
                    if (!data.success) {
                        console.error('Upload error:', data.error, 'path:', path, 'file:', file.name);
                    }
                } catch (e) {
                    console.error('Upload parse error:', xhr.responseText);
                }
                resolve();
            };

            xhr.onerror = () => {
                console.error('Upload network error:', file.name, 'to', path);
                resolve();
            };

            xhr.send(formData);
        });
    }

    showProgress(fileName) {
        const el = document.getElementById('upload-progress');
        el.classList.add('active');
        document.getElementById('progress-text').textContent = `Uploading ${fileName}...`;
        document.getElementById('progress-bar').style.width = '0%';
    }

    updateProgress(pct, fileName) {
        document.getElementById('progress-bar').style.width = pct + '%';
        document.getElementById('progress-text').textContent = `Uploading ${fileName} — ${pct}%`;
    }

    hideProgress() {
        document.getElementById('upload-progress').classList.remove('active');
    }

    downloadFile(path) {
        const link = document.createElement('a');
        link.href = `api/files/download?path=${encodeURIComponent(path)}`;
        link.download = '';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    }

    downloadFolder(path) {
        const link = document.createElement('a');
        link.href = `api/files/download-folder?path=${encodeURIComponent(path)}`;
        link.download = '';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    }

    async createNewFolder() {
        const name = prompt('Enter folder name:');
        if (name) {
            try {
                const response = await fetch('api/files/mkdir', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': this.token,
                    },
                    body: JSON.stringify({
                        path: this.currentPath,
                        name: name
                    })
                });

                const data = await response.json();
                if (data.success) {
                    this.loadFiles(this.currentPath);
                } else {
                    alert(data.error || 'Failed to create folder');
                }
            } catch (error) {
                console.error('Create folder error:', error);
                alert('Failed to create folder');
            }
        }
    }

    async deleteItem(path, name, isDir) {
        const type = isDir ? 'folder' : 'file';
        const confirmMsg = isDir
            ? `Delete folder "${name}" and all its contents?`
            : `Delete file "${name}"?`;

        if (!confirm(confirmMsg)) {
            return;
        }

        try {
            const response = await fetch('api/files/delete', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': this.token,
                },
                body: JSON.stringify({ path })
            });

            const data = await response.json();
            if (data.success) {
                this.loadFiles(this.currentPath);
            } else {
                alert(data.error || 'Failed to delete');
            }
        } catch (error) {
            console.error('Delete error:', error);
            alert('Failed to delete');
        }
    }

    // Cookie helpers
    setCookie(name, value, days) {
        const expires = new Date(Date.now() + days * 864e5).toUTCString();
        document.cookie = `${name}=${encodeURIComponent(value)}; expires=${expires}; path=/`;
    }

    getCookie(name) {
        return document.cookie.split('; ').reduce((r, v) => {
            const parts = v.split('=');
            return parts[0] === name ? decodeURIComponent(parts[1]) : r;
        }, '');
    }
}

// Initialize app
const app = new App();
