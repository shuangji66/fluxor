window.API = (function() {
    const BASE = window.BASE_URL || '';
    
    function withBase(path) {
        if (!BASE || BASE === '/') return path;
        return BASE.replace(/\/$/, '') + '/' + path.replace(/^\//, '');
    }
    
    async function apiFetch(path, options = {}) {
        try {
            const url = withBase(path);
            const resp = await fetch(url, options);
            if (!resp.ok) {
                // 仅保留必要的警告，但不输出日志
            }
            return resp;
        } catch (err) {
            throw new Error('网络错误: ' + err.message);
        }
    }
    
    function wsConnect(path, onMessage, handlers = {}) {
        const wsUrl = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + withBase(path);
        const ws = new WebSocket(wsUrl);
        
        ws.onopen = () => {
            if (handlers.onOpen) handlers.onOpen();
        };
        
        ws.onmessage = (e) => {
            try {
                onMessage(e);
            } catch (err) {
                // 仅记录错误，不输出日志
            }
        };
        
        ws.onerror = (e) => {
            if (handlers.onError) handlers.onError(e);
        };
        
        ws.onclose = (e) => {
            if (handlers.onClose) handlers.onClose(e);
        };
        
        return ws;
    }
    
    return { apiFetch, wsConnect };
})();