class App {
    static START_PATH = '/root/';
    static AUTOCOMPLETE_DELAY = 200;
    static PANEL_TRANSITION = 320;
    static MOBILE_WIDTH = 768;

    constructor() {
        this.settings = new Settings('webterminal.');
        this.session = new Session(sessionStorage);
        this.api = new ApiClient(this.session, () => this.teardown());

        this.elements = App.collectElements();
        this.currentPath = '/';
        this.suggestionIndex = -1;
        this.suggestTimer = null;
        this.terminal = null;
        this.perf = null;

        this.fileList = new FileListView(this.elements.fileList, {
            onOpen: (path) => this.loadFiles(path),
            onDownload: (path, isDir) => this.download(path, isDir),
            onDelete: (path, name, isDir) => this.deleteItem(path, name, isDir),
            onInsertPath: (path) => this.terminal?.send(path)
        });
        this.fileList.setSort(
            this.settings.get('sortField', 'name'),
            this.settings.get('sortAsc', 'true') !== 'false'
        );

        this.bindEvents();
        this.updateSortUI();

        if (this.session.token) this.showApp();
        else this.showLogin();
    }

    static collectElements() {
        const ids = [
            'login-screen', 'app', 'login-form', 'login', 'password', 'login-error',
            'logout-btn', 'user-info', 'refresh-files', 'new-folder', 'current-path',
            'path-autocomplete', 'file-list', 'file-input', 'upload-area',
            'upload-progress', 'progress-bar', 'progress-text', 'clear-terminal',
            'toggle-files', 'panel-overlay', 'terminal', 'terminal-keys',
            'cpu-graph', 'cpu-text', 'mem-text', 'net-text'
        ];
        const camel = (id) => id.replace(/-(\w)/g, (_, c) => c.toUpperCase());
        return Object.fromEntries(ids.map((id) => [camel(id), document.getElementById(id)]));
    }

    bindEvents() {
        const el = this.elements;

        el.loginForm.addEventListener('submit', (event) => {
            event.preventDefault();
            this.login();
        });
        el.logoutBtn.addEventListener('click', () => this.logout());

        el.refreshFiles.addEventListener('click', () => this.loadFiles(this.currentPath));
        el.newFolder.addEventListener('click', () => this.createFolder());
        el.clearTerminal.addEventListener('click', () => this.terminal?.clear());

        for (const field of ['name', 'size', 'date']) {
            document.getElementById(`sort-${field}`)
                .addEventListener('click', () => this.toggleSort(field));
        }

        el.currentPath.addEventListener('keydown', (event) => this.onPathKey(event));
        el.currentPath.addEventListener('input', () => this.onPathInput());
        el.currentPath.addEventListener('focus', () => {
            if (el.pathAutocomplete.childElementCount && el.currentPath.value.trim()) {
                el.pathAutocomplete.classList.add('active');
            }
        });
        el.pathAutocomplete.addEventListener('click', (event) => {
            const suggestion = event.target.closest('.path-suggestion');
            if (!suggestion) return;
            el.currentPath.value = suggestion.dataset.path;
            this.closeSuggestions();
            this.loadFiles(suggestion.dataset.path);
        });
        document.addEventListener('click', (event) => {
            if (!event.target.closest('.file-browser-path')) this.closeSuggestions();
        });

        el.fileInput.addEventListener('change', (event) => {
            this.uploadAll([...event.target.files].map((file) => ({ file, path: this.currentPath })));
            event.target.value = '';
        });

        for (const type of ['dragover', 'dragleave', 'drop']) {
            el.uploadArea.addEventListener(type, (event) => this.onDrag(type, event));
        }

        el.toggleFiles.addEventListener('click', () => this.toggleFilePanel());
        el.panelOverlay.addEventListener('click', () => this.closeFilePanel());
    }

    showLogin() {
        this.elements.loginScreen.style.display = 'flex';
        this.elements.app.style.display = 'none';
    }

    showApp() {
        this.elements.loginScreen.style.display = 'none';
        this.elements.app.style.display = 'flex';
        this.elements.userInfo.textContent = this.session.user;

        this.terminal = new TerminalManager(this.session);
        this.terminal.init(this.elements.terminal, this.elements.terminalKeys);

        this.perf = new PerfWidget(this.api, {
            canvas: this.elements.cpuGraph,
            cpu: this.elements.cpuText,
            memory: this.elements.memText,
            network: this.elements.netText
        });
        this.perf.start();

        this.loadFiles(App.START_PATH);
    }

    async login() {
        const { loginError } = this.elements;
        try {
            const data = await this.api.post('api/login', {
                login: this.elements.login.value,
                password: this.elements.password.value
            });
            this.session.start(data.token, data.user);
            loginError.style.display = 'none';
            this.elements.password.value = '';
            this.showApp();
        } catch (error) {
            loginError.textContent = error.message;
            loginError.style.display = 'block';
        }
    }

    async logout() {
        try {
            await this.api.signal('api/logout');
        } catch (error) {
            console.error('Logout error:', error);
        }
        this.teardown();
    }

    teardown() {
        if (!this.session.token) return;

        this.perf?.stop();
        this.terminal?.disconnect();
        this.perf = null;
        this.terminal = null;

        this.session.clear();
        this.showLogin();
    }

    async loadFiles(path) {
        try {
            const data = await this.api.get('api/files', { path });
            this.currentPath = data.path;
            this.elements.currentPath.value = data.path;
            this.fileList.setEntries(data.path, data.files);
            this.fileList.render();
            if (window.innerWidth <= App.MOBILE_WIDTH) this.closeFilePanel();
        } catch (error) {
            this.report(error);
        }
    }

    onPathKey(event) {
        const { currentPath, pathAutocomplete } = this.elements;

        if (event.key === 'Enter') {
            event.preventDefault();
            this.closeSuggestions();
            this.loadFiles(currentPath.value || '/');
            return;
        }
        if (event.key === 'Escape') {
            this.closeSuggestions();
            return;
        }

        const items = pathAutocomplete.children;
        if (!items.length) return;

        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            const delta = event.key === 'ArrowDown' ? 1 : -1;
            this.highlight(Math.min(Math.max(this.suggestionIndex + delta, 0), items.length - 1));
            return;
        }
        if (event.key === 'Tab' && this.suggestionIndex >= 0) {
            event.preventDefault();
            currentPath.value = items[this.suggestionIndex].dataset.path;
            this.closeSuggestions();
        }
    }

    onPathInput() {
        clearTimeout(this.suggestTimer);
        this.suggestionIndex = -1;

        const value = this.elements.currentPath.value.trim();
        if (!value) {
            this.closeSuggestions();
            return;
        }
        this.suggestTimer = setTimeout(() => this.suggest(value), App.AUTOCOMPLETE_DELAY);
    }

    highlight(index) {
        this.suggestionIndex = index;
        const items = this.elements.pathAutocomplete.children;
        for (let i = 0; i < items.length; i++) {
            items[i].classList.toggle('selected', i === index);
        }
    }

    closeSuggestions() {
        this.suggestionIndex = -1;
        this.elements.pathAutocomplete.classList.remove('active');
    }

    async suggest(input) {
        const separator = input.lastIndexOf('/');
        const parent = separator < 0 ? '/' : input.slice(0, separator) || '/';
        const prefix = input.slice(separator + 1).toLowerCase();

        let data;
        try {
            data = await this.api.get('api/files', { path: parent });
        } catch (error) {
            this.closeSuggestions();
            return;
        }

        const matches = data.files
            .filter((entry) => entry.isDir && entry.name.toLowerCase().startsWith(prefix))
            .sort((a, b) => a.name.localeCompare(b.name));

        if (!matches.length) {
            this.closeSuggestions();
            return;
        }

        const fragment = document.createDocumentFragment();
        for (const entry of matches) {
            const suggestion = document.createElement('div');
            suggestion.className = 'path-suggestion';
            suggestion.dataset.path = `${entry.path}/`;
            suggestion.textContent = `${entry.path}/`;
            fragment.appendChild(suggestion);
        }

        this.elements.pathAutocomplete.replaceChildren(fragment);
        this.elements.pathAutocomplete.classList.add('active');
        this.suggestionIndex = -1;
    }

    toggleSort(field) {
        const ascending = this.fileList.field === field ? !this.fileList.ascending : true;
        this.fileList.setSort(field, ascending);

        this.settings.set('sortField', field);
        this.settings.set('sortAsc', ascending);

        this.updateSortUI();
        this.fileList.render();
    }

    updateSortUI() {
        for (const button of document.querySelectorAll('.sort-btn')) {
            button.classList.remove('active', 'asc', 'desc');
        }
        const active = document.getElementById(`sort-${this.fileList.field}`);
        active?.classList.add('active', this.fileList.ascending ? 'asc' : 'desc');
    }

    onDrag(type, event) {
        event.preventDefault();
        event.stopPropagation();

        const { uploadArea } = this.elements;
        if (type === 'dragover') {
            uploadArea.classList.add('dragover');
            return;
        }

        uploadArea.classList.remove('dragover');
        if (type === 'drop') this.uploadDrop(event.dataTransfer);
    }

    async uploadDrop(transfer) {
        if (!transfer.items) {
            this.uploadAll([...transfer.files].map((file) => ({ file, path: this.currentPath })));
            return;
        }

        const queue = [];
        for (const item of transfer.items) {
            if (item.kind !== 'file') continue;
            const entry = item.webkitGetAsEntry?.();
            if (entry) queue.push({ entry, path: this.currentPath });
            else {
                const file = item.getAsFile();
                if (file) queue.push({ file, path: this.currentPath });
            }
        }

        this.uploadAll(await App.flatten(queue));
    }

    static async flatten(queue) {
        const files = [];

        while (queue.length) {
            const { entry, file, path } = queue.shift();
            if (file) {
                files.push({ file, path });
                continue;
            }

            if (entry.isFile) {
                files.push({ file: await new Promise((resolve) => entry.file(resolve)), path });
                continue;
            }

            const childPath = path === '/' ? `/${entry.name}` : `${path}/${entry.name}`;
            for (const child of await App.readDirectory(entry)) {
                queue.push({ entry: child, path: childPath });
            }
        }

        return files;
    }

    static readDirectory(entry) {
        const reader = entry.createReader();
        const entries = [];
        return new Promise((resolve) => {
            const readBatch = () => reader.readEntries((batch) => {
                if (!batch.length) resolve(entries);
                else {
                    entries.push(...batch);
                    readBatch();
                }
            });
            readBatch();
        });
    }

    async uploadAll(files) {
        if (!files.length) return;

        this.elements.uploadProgress.classList.add('active');
        const failures = [];

        for (let i = 0; i < files.length; i++) {
            const { file, path } = files[i];
            const label = `${i + 1}/${files.length}: ${file.name}`;
            this.setProgress(0, label);
            try {
                await this.api.upload('api/files/upload', file, path,
                    (percent) => this.setProgress(percent, label));
            } catch (error) {
                failures.push(`${file.name}: ${error.message}`);
            }
        }

        this.elements.uploadProgress.classList.remove('active');
        if (failures.length) alert(`Upload failed:\n${failures.join('\n')}`);
        this.loadFiles(this.currentPath);
    }

    setProgress(percent, label) {
        this.elements.progressBar.style.width = `${percent}%`;
        this.elements.progressText.textContent = `Uploading ${label} — ${percent}%`;
    }

    toggleFilePanel() {
        const panel = document.querySelector('.file-browser');
        if (panel.classList.contains('open')) {
            this.closeFilePanel();
            return;
        }
        panel.classList.add('open');
        this.elements.panelOverlay.classList.add('active');
    }

    closeFilePanel() {
        document.querySelector('.file-browser').classList.remove('open');
        this.elements.panelOverlay.classList.remove('active');
        this.terminal?.scheduleFit(App.PANEL_TRANSITION);
    }

    download(path, isDir) {
        const endpoint = isDir ? 'api/files/download-folder' : 'api/files/download';
        const link = document.createElement('a');
        link.href = this.api.downloadUrl(endpoint, { path });
        link.download = '';
        document.body.appendChild(link);
        link.click();
        link.remove();
    }

    async createFolder() {
        const name = prompt('Enter folder name:');
        if (!name) return;

        try {
            await this.api.post('api/files/mkdir', { path: this.currentPath, name });
            this.loadFiles(this.currentPath);
        } catch (error) {
            this.report(error);
        }
    }

    async deleteItem(path, name, isDir) {
        const question = isDir
            ? `Delete folder "${name}" and all its contents?`
            : `Delete file "${name}"?`;
        if (!confirm(question)) return;

        try {
            await this.api.post('api/files/delete', { path });
            this.loadFiles(this.currentPath);
        } catch (error) {
            this.report(error);
        }
    }

    report(error) {
        console.error(error);
        alert(error.message);
    }
}

new App();
