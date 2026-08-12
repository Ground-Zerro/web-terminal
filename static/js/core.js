const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];
const BYTE_STEP = 1024;

function formatBytes(bytes, suffix = '') {
    if (!(bytes > 0)) return `0 B${suffix}`;
    const unit = Math.min(
        Math.floor(Math.log(bytes) / Math.log(BYTE_STEP)),
        BYTE_UNITS.length - 1
    );
    const value = bytes / BYTE_STEP ** unit;
    return `${value.toFixed(unit === 0 ? 0 : 1)} ${BYTE_UNITS[unit]}${suffix}`;
}

class Settings {
    constructor(prefix) {
        this.prefix = prefix;
    }

    get(key, fallback) {
        const stored = localStorage.getItem(this.prefix + key);
        return stored === null ? fallback : stored;
    }

    set(key, value) {
        localStorage.setItem(this.prefix + key, String(value));
    }
}

class Session {
    constructor(storage) {
        this.storage = storage;
    }

    get token() {
        return this.storage.getItem('token');
    }

    get user() {
        return this.storage.getItem('user') || '';
    }

    start(token, user) {
        this.storage.setItem('token', token);
        this.storage.setItem('user', user);
    }

    clear() {
        this.storage.removeItem('token');
        this.storage.removeItem('user');
    }
}

class ApiClient {
    constructor(session, onUnauthorized) {
        this.session = session;
        this.onUnauthorized = onUnauthorized;
    }

    get(path, params) {
        return this.send(this.url(path, params));
    }

    post(path, body) {
        return this.send(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
    }

    signal(path) {
        return this.send(path, { method: 'POST' });
    }

    url(path, params) {
        if (!params) return path;
        return `${path}?${new URLSearchParams(params)}`;
    }

    downloadUrl(path, params) {
        return this.url(path, { ...params, token: this.session.token });
    }

    async send(path, options = {}) {
        const token = this.session.token;
        const headers = { ...options.headers };
        if (token) headers.Authorization = token;

        const response = await fetch(path, { ...options, headers });
        const data = await response.json().catch(() => null);

        if (response.status === 401 && token) this.onUnauthorized();

        if (!data) throw new Error(`Unexpected response (HTTP ${response.status})`);
        if (!data.success) throw new Error(data.error || `HTTP ${response.status}`);
        return data;
    }

    upload(path, file, targetPath, onProgress) {
        return new Promise((resolve, reject) => {
            const form = new FormData();
            form.append('file', file);
            form.append('path', targetPath);

            const request = new XMLHttpRequest();
            request.open('POST', path);
            request.setRequestHeader('Authorization', this.session.token);

            request.upload.onprogress = (event) => {
                if (event.lengthComputable) {
                    onProgress(Math.round((event.loaded / event.total) * 100));
                }
            };

            request.onload = () => {
                let data = null;
                try {
                    data = JSON.parse(request.responseText);
                } catch (error) {
                    reject(new Error(`Malformed response (HTTP ${request.status})`));
                    return;
                }
                if (data.success) resolve(data);
                else reject(new Error(data.error || `HTTP ${request.status}`));
            };

            request.onerror = () => reject(new Error('Network error'));
            request.send(form);
        });
    }
}
