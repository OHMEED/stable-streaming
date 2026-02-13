import React from 'react';
import { Card } from '../components/ui/Card';
import api from '../lib/api';

const OBSPage: React.FC = () => {
    const [obsConnected, setObsConnected] = React.useState(false);
    const [iframeKey, setIframeKey] = React.useState(0);
    const [vncPassword, setVncPassword] = React.useState<string | null>(null);
    const [vncToken, setVncToken] = React.useState<string | null>(null);
    // Server-validated websockify path (from Portal API, not client-side JWT decode)
    const [validatedWebsockifyPath, setValidatedWebsockifyPath] = React.useState<string | null>(null);
    // Cloudflare direct URL for low-latency connection (bypasses Portal proxy)
    const [directWebsockifyUrl, setDirectWebsockifyUrl] = React.useState<string | null>(null);
    const prevObsConnected = React.useRef(false);

    // Fetch VNC credentials on mount
    // Priority: 1) Direct Cloudflare URL, 2) Portal validated path, 3) Local fallback
    React.useEffect(() => {
        const fetchVNCCredentials = async () => {
            // Fetch VNC password from backend
            try {
                const passwordRes = await api.get('/vnc/password');
                setVncPassword(passwordRes.data.password);
            } catch (e) {
                console.error("Failed to fetch VNC password", e);
            }

            // Fetch auth token from backend (for WebSocket auth)
            try {
                const tokenRes = await api.get('/vnc/token');
                setVncToken(tokenRes.data.token);
            } catch (e) {
                console.error("Failed to fetch VNC token", e);
                // Fallback to localStorage token for local login
                const localToken = localStorage.getItem('token');
                if (localToken) {
                    setVncToken(localToken);
                }
            }

            // Try to get direct Cloudflare URL (lowest latency, bypasses Portal)
            // This is set up via Cloudflare for SaaS with each server having its own hostname
            try {
                const directRes = await api.get('/vnc/direct-url');
                if (directRes.data.available && directRes.data.directUrl) {
                    console.log("Using direct Cloudflare connection:", directRes.data.hostname);
                    setDirectWebsockifyUrl(directRes.data.directUrl);
                    if (directRes.data.token) {
                        setVncToken(directRes.data.token);
                    }
                    return; // Direct URL available, no need for Portal path
                }
            } catch (e) {
                // Cloudflare not configured, fall through to Portal path
            }

            // Fallback: Fetch validated websockify path from Portal API
            // This goes through Portal proxy with SSRF protection
            try {
                const pathRes = await fetch('/api/vnc/websockify-path', {
                    credentials: 'include' // Include auth_token cookie
                });
                if (pathRes.ok) {
                    const data = await pathRes.json();
                    setValidatedWebsockifyPath(data.path);
                    if (data.token) {
                        setVncToken(data.token);
                    }
                }
            } catch (e) {
                console.error("Failed to fetch websockify path", e);
            }
        };
        fetchVNCCredentials();
    }, []);

    // Poll OBS Status
    React.useEffect(() => {
        const checkStatus = async () => {
            try {
                const res = await api.get('/obs/status');
                setObsConnected(res.data.connected);
            } catch (e) {
                console.error("Failed to check OBS status", e);
                setObsConnected(false);
            }
        };

        const interval = setInterval(checkStatus, 3000);
        checkStatus();

        return () => clearInterval(interval);
    }, []);

    // Auto-reload iframe when OBS connects (fixes white screen/disconnect on launch)
    React.useEffect(() => {
        if (!prevObsConnected.current && obsConnected) {
            console.log("OBS came online, reloading VNC viewer...");
            setIframeKey(k => k + 1);
        }
        prevObsConnected.current = obsConnected;
    }, [obsConnected]);

    // noVNC compression settings for bandwidth optimization
    // quality: 0-9 (0=worst, 9=best) - lower = smaller data
    // compression: 0-9 (9=max compression)
    const compressionParams = 'quality=6&compression=9';

    // Build iframe URL - prioritize direct Cloudflare URL for lowest latency
    // Priority: 1) Direct Cloudflare URL, 2) Portal proxy path, 3) Local fallback
    const iframeSrc = (() => {
        const baseParams = `autoconnect=true&resize=scale&reconnect=true&reconnect_delay=1000&${compressionParams}`;
        const passwordParam = vncPassword ? `&password=${encodeURIComponent(vncPassword)}` : '';

        // 1) BEST: Direct Cloudflare connection (lowest latency, bypasses Portal)
        if (directWebsockifyUrl) {
            // Direct URL is absolute: wss://example.com/api/vnc/websockify?token=...
            // We load noVNC from the direct hostname
            const hostname = new URL(directWebsockifyUrl).hostname;
            const directPath = directWebsockifyUrl.replace(/^wss?:\/\/[^/]+/, '');
            // noVNC path parameter needs the full path including query string
            return `https://${hostname}/api/vnc/vnc.html?${baseParams}&path=${encodeURIComponent(directPath.slice(1))}${passwordParam}`;
        }

        // 2) Portal proxy path (validated server-side with SSRF protection)
        if (validatedWebsockifyPath) {
            const tokenParam = vncToken ? `?token=${encodeURIComponent(vncToken)}` : '';
            const path = `${validatedWebsockifyPath}${tokenParam}`;
            return `/server/api/vnc/vnc.html?${baseParams}&path=${encodeURIComponent(path)}${passwordParam}`;
        }

        // 3) Local fallback (direct backend access without Portal)
        const localPath = vncToken
            ? `server/api/vnc/websockify?token=${encodeURIComponent(vncToken)}`
            : 'server/api/vnc/websockify';
        return `/server/api/vnc/vnc.html?${baseParams}&path=${localPath}${passwordParam}`;
    })();

    return (
        <div className="h-full w-full flex flex-col">
            {/* Edge-to-edge noVNC Container */}
            <Card className="flex-1 flex flex-col p-0 overflow-hidden relative border-0 rounded-none dark:bg-black bg-dark-900">
                <div className="dark:bg-black bg-dark-900 flex-1 relative w-full h-full">
                    {/* noVNC connects through JWT-protected backend proxy to wayvnc (Wayland) */}
                    <iframe
                        key={iframeKey}
                        src={iframeSrc}
                        className="w-full h-full border-none"
                        title="OBS Remote View"
                        allow="clipboard-read; clipboard-write; fullscreen"
                    />
                </div>

                <div className="dark:bg-dark-900/90 bg-dark-800/90 p-2 border-t dark:border-dark-700 border-dark-600 backdrop-blur-sm shrink-0">
                    <div className="flex items-center justify-between px-2">
                        <div className="flex items-center gap-2 text-xs dark:text-dark-300 text-dark-200">
                            <div className={`w-2 h-2 rounded-full ${obsConnected ? 'bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)]' : 'bg-red-500'} transition-colors duration-500`}></div>
                            {obsConnected ? 'Connected to OBS (Wayland VNC)' : 'Waiting for OBS...'}
                        </div>

                        <div className="flex items-center gap-2">
                            <input
                                type="file"
                                id="file-upload"
                                className="hidden"
                                onChange={async (e) => {
                                    const file = e.target.files?.[0];
                                    if (!file) return;

                                    const formData = new FormData();
                                    formData.append('file', file);

                                    try {
                                        // For now, rely on cookie or basic fetch
                                        const res = await fetch('/api/upload', {
                                            method: 'POST',
                                            body: formData,
                                        });

                                        if (res.ok) {
                                            alert(`File ${file.name} uploaded successfully to ~/Downloads/uploads`);
                                        } else {
                                            alert('Upload failed');
                                        }
                                    } catch (err) {
                                        console.error(err);
                                        alert('Upload error');
                                    }

                                    // Reset input
                                    e.target.value = '';
                                }}
                            />
                            <label
                                htmlFor="file-upload"
                                className="cursor-pointer px-3 py-1 bg-accent-cyan hover:bg-accent-cyan-dark text-dark-900 text-xs rounded flex items-center gap-2 transition-colors font-medium"
                            >
                                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                                    <polyline points="17 8 12 3 7 8" />
                                    <line x1="12" y1="3" x2="12" y2="15" />
                                </svg>
                                Upload File
                            </label>
                        </div>
                    </div>
                </div>
            </Card>
        </div>
    );
};

export default OBSPage;
