class FileListView {
    static ICONS = {
        js: '📜', ts: '📜', py: '🐍', go: '🔵', rs: '🦀', java: '☕',
        c: '⚙️', cpp: '⚙️', h: '📄', html: '🌐', css: '🎨',
        json: '📋', xml: '📋', yaml: '📋', yml: '📋', log: '📋',
        md: '📝', txt: '📝', sh: '🐚', bash: '🐚',
        zip: '📦', tar: '📦', gz: '📦',
        jpg: '🖼️', jpeg: '🖼️', png: '🖼️', gif: '🖼️', svg: '🖼️',
        mp3: '🎵', mp4: '🎬',
        pdf: '📄', doc: '📄', docx: '📄', xls: '📊', xlsx: '📊'
    };

    static FILE_ICON = '📄';
    static DIR_ICON = '📁';
    static PARENT = '..';

    static COMPARATORS = {
        name: (a, b) => a.name.localeCompare(b.name),
        size: (a, b) => (a.size || 0) - (b.size || 0),
        date: (a, b) => (a.modTime || '').localeCompare(b.modTime || '')
    };

    constructor(root, actions) {
        this.root = root;
        this.actions = actions;
        this.entries = [];
        this.path = '/';
        this.field = 'name';
        this.ascending = true;

        this.root.addEventListener('click', (event) => this.onClick(event));
        this.root.addEventListener('mousedown', (event) => this.onMiddleClick(event));
    }

    setSort(field, ascending) {
        this.field = field;
        this.ascending = ascending;
    }

    setEntries(path, entries) {
        this.path = path;
        this.entries = entries;
    }

    onClick(event) {
        const item = event.target.closest('.file-item');
        if (!item) return;

        const { path, dir } = item.dataset;
        const isDir = dir === 'true';

        switch (event.target.dataset.action) {
            case 'download':
                this.actions.onDownload(path, isDir);
                return;
            case 'delete':
                this.actions.onDelete(path, item.dataset.name, isDir);
                return;
            default:
                if (isDir) this.actions.onOpen(path);
        }
    }

    onMiddleClick(event) {
        if (event.button !== 1) return;
        const item = event.target.closest('.file-item');
        if (!item) return;

        event.preventDefault();
        this.actions.onInsertPath(item.dataset.path + (item.dataset.dir === 'true' ? '/' : ''));
    }

    sorted() {
        const compare = FileListView.COMPARATORS[this.field];
        const direction = this.ascending ? 1 : -1;
        return this.entries.slice().sort((a, b) => {
            if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
            return compare(a, b) * direction;
        });
    }

    render() {
        const fragment = document.createDocumentFragment();

        if (this.path !== '/') {
            const parent = this.path.slice(0, this.path.lastIndexOf('/')) || '/';
            fragment.appendChild(this.createItem({
                name: FileListView.PARENT,
                path: parent,
                isDir: true
            }, false));
        }

        for (const entry of this.sorted()) {
            fragment.appendChild(this.createItem(entry, true));
        }

        this.root.replaceChildren(fragment);
    }

    createItem(entry, withActions) {
        const item = document.createElement('div');
        item.className = 'file-item';
        item.dataset.path = entry.path;
        item.dataset.name = entry.name;
        item.dataset.dir = String(entry.isDir);

        const icon = document.createElement('span');
        icon.className = 'file-icon';
        icon.textContent = entry.isDir ? FileListView.DIR_ICON : FileListView.iconFor(entry.name);
        item.appendChild(icon);

        const info = document.createElement('div');
        info.className = 'file-info';

        const name = document.createElement('div');
        name.className = 'file-name';
        name.title = entry.name;
        name.textContent = entry.name;
        info.appendChild(name);

        const meta = document.createElement('div');
        meta.className = 'file-meta';
        meta.textContent = entry.isDir
            ? (entry.modTime || '')
            : `${formatBytes(entry.size)} ${entry.modTime || ''}`.trim();
        info.appendChild(meta);

        item.appendChild(info);

        if (withActions) {
            item.appendChild(this.createActions(entry.isDir));
        }
        return item;
    }

    createActions(isDir) {
        const actions = document.createElement('div');
        actions.className = 'file-actions';

        const download = document.createElement('button');
        download.className = 'btn-download';
        download.dataset.action = 'download';
        download.title = isDir ? 'Download as ZIP' : 'Download';
        download.textContent = isDir ? '📦' : '⬇';
        actions.appendChild(download);

        const remove = document.createElement('button');
        remove.className = 'btn-delete';
        remove.dataset.action = 'delete';
        remove.title = 'Delete';
        remove.textContent = '🗑️';
        actions.appendChild(remove);

        return actions;
    }

    static iconFor(filename) {
        const dot = filename.lastIndexOf('.');
        if (dot <= 0) return FileListView.FILE_ICON;
        return FileListView.ICONS[filename.slice(dot + 1).toLowerCase()] || FileListView.FILE_ICON;
    }
}
