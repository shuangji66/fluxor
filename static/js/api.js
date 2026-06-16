window.API = (function() {
    const BASE = window.BASE_URL || '';
    function withBase(path) {
        if (!BASE || BASE === '/') return path;
        return BASE.replace(/\/$/, '') + '/' + path.replace(/^\//, '');
    }
    async function apiFetch(path, options) {
        try {
            const resp = await fetch(withBase(path), options);
            return resp;
        } catch (err) {
            throw new Error('网络错误: ' + err.message);
        }
    }
    function wsConnect(path, onMessage) {
        const wsUrl = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + withBase(path);
        const ws = new WebSocket(wsUrl);
        ws.onmessage = onMessage;
        ws.onerror = (e) => console.error('WebSocket error', e);
        ws.onclose = () => console.log('WebSocket closed');
        return ws;
    }
    return { apiFetch, wsConnect };
})();