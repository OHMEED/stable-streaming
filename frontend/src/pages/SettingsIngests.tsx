import React, { useEffect, useState } from 'react';
import api from '../lib/api';
import { Card } from '../components/ui/Card';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';


interface Ingest {
    id: number;
    name: string;
    protocol: 'rtmp' | 'srt' | 'srtla';
    port: number;
    output_port: number;
    srt_port: number; // For SRTLA internal relay
    bs_port: number;  // For SRTLA broadcaster
    ws_port: number;  // For stats
    rtsp_port: number; // For SRTLA RTSP egress
    stream_key: string;
    stream_id: string; // For SRT/SRTLA stream id
    enabled: boolean;
    is_running: boolean;
}



import { IngestStatus } from '../components/IngestStatus';

const SettingsIngests: React.FC = () => {
    const [ingests, setIngests] = useState<Ingest[]>([]);
    // Removed unused states for clean build
    // const [loading, setLoading] = useState(true);
    // const [showAddModal, setShowAddModal] = useState(false);
    // const [stats, setStats] = useState<Record<number, any>>({});

    // Server public IP for external connection URLs
    const [serverIP, setServerIP] = useState(window.location.hostname);
    const [formData, setFormData] = useState({
        name: '',
        protocol: 'srtla',
        password: '',
    });
    const [isLoading, setIsLoading] = useState(false);
    const [obsScenes, setObsScenes] = useState<string[]>([]);
    const [selectedScenes, setSelectedScenes] = useState<Record<number, string>>({});
    // Track "Added" state - persisted across reboot by checking OBS on load
    const [addedSources, setAddedSources] = useState<Record<string, boolean>>({});
    // Ingest lock state - when true, hide Add New Ingest panel
    const [ingestsLocked, setIngestsLocked] = useState(false);
    // Rename state
    const [editingIngestId, setEditingIngestId] = useState<number | null>(null);
    const [editingName, setEditingName] = useState('');

    // Fetch server public IP on mount
    const fetchServerInfo = async () => {
        try {
            const res = await api.get('/server/info');
            if (res.data.public_ip) {
                setServerIP(res.data.public_ip);
            }
        } catch (err) {
            console.error("Failed to fetch server info", err);
            // Keep using window.location.hostname as fallback
        }
    };

    // Fetch ingest lock status
    const fetchIngestsLocked = async () => {
        try {
            const res = await api.get('/settings/ingests-locked');
            setIngestsLocked(res.data.locked || false);
        } catch (err) {
            console.error("Failed to fetch ingests lock status", err);
        }
    };

    useEffect(() => {
        const init = async () => {
            await fetchServerInfo(); // Fetch public IP first
            await fetchIngestsLocked(); // Fetch lock status
            const [ingestList, sceneList] = await Promise.all([
                fetchIngests(),
                fetchScenes()
            ]);
            // Check existing sources in OBS to persist "Added" state
            if (sceneList.length > 0 && ingestList.length > 0) {
                await checkExistingSources(sceneList, ingestList);
            }
        };
        init();
    }, []);

    // Check existing sources in OBS scenes to persist "Added" state across reboot
    const checkExistingSources = async (scenes: string[], ingestList: Ingest[]) => {
        const existingSources: Record<string, boolean> = {};

        for (const scene of scenes) {
            try {
                const res = await api.get(`/obs/scene/${encodeURIComponent(scene)}/items`);
                const items: string[] = res.data || [];

                // Check each ingest to see if its source exists in this scene
                for (const ingest of ingestList) {
                    const sourceName = `${ingest.name}_MEDIA_SOURCE`;
                    if (items.includes(sourceName)) {
                        existingSources[`${ingest.id}-${scene}`] = true;
                    }
                }
            } catch (err) {
                // Scene might not exist or OBS not connected, skip silently
            }
        }

        setAddedSources(existingSources);
    };

    const fetchScenes = async () => {
        try {
            const res = await api.get('/obs/scenes');
            if (res.data && Array.isArray(res.data)) {
                setObsScenes(res.data);
                return res.data;
            }
        } catch (err) {
            console.error("Failed to fetch scenes", err);
        }
        return [];
    };

    const fetchIngests = async (): Promise<Ingest[]> => {
        try {
            const res = await api.get('/ingests');
            const data = res.data || [];
            setIngests(data);
            return data;
        } catch (err) {
            console.error("Failed to fetch ingests", err);
            return [];
        }
    };

    const handleDelete = async (id: number) => {
        if (!confirm('Are you sure you want to delete this ingest configuration?')) return;
        try {
            await api.delete(`/ingests/${id}`);
            fetchIngests();
        } catch (err) {
            console.error(err);
        }
    };

    // Rename ingest
    const handleStartEdit = (ingest: Ingest) => {
        setEditingIngestId(ingest.id);
        setEditingName(ingest.name);
    };

    const handleCancelEdit = () => {
        setEditingIngestId(null);
        setEditingName('');
    };

    const handleSaveRename = async () => {
        if (!editingIngestId || !editingName.trim()) return;
        try {
            await api.patch(`/ingests/${editingIngestId}/name`, { name: editingName.trim() });
            // Update local state
            setIngests(prev => prev.map(ing =>
                ing.id === editingIngestId ? { ...ing, name: editingName.trim() } : ing
            ));
            setEditingIngestId(null);
            setEditingName('');
        } catch (err) {
            console.error('Failed to rename ingest', err);
            alert('Failed to rename ingest');
        }
    };

    // Generate a secure random password (20 chars, alphanumeric only)
    const generatePassword = () => {
        const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
        let password = '';
        const array = new Uint32Array(20);
        crypto.getRandomValues(array);
        for (let i = 0; i < 20; i++) {
            password += chars[array[i] % chars.length];
        }
        setFormData({ ...formData, password });
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        // Password is now mandatory for ALL protocols including SRT
        // For SRT it is used for path-based authentication (publish:PASSWORD/stream)
        if (formData.password.length < 10) {
            alert('Password must be at least 10 characters');
            return;
        }
        setIsLoading(true);
        try {
            // Send password as stream_key (backend uses this field for all protocols)
            await api.post('/ingests', {
                name: formData.name,
                protocol: formData.protocol,
                stream_key: formData.password,
                stream_id: '', // Not used anymore
            });
            setFormData({ name: '', protocol: 'srtla', password: '' });
            fetchIngests();
        } catch (err) {
            console.error(err);
        } finally {
            setIsLoading(false);
        }
    };

    const getConnectionString = (ingest: Ingest) => {
        // Use public IP from backend for external connections
        const ip = serverIP;
        if (ingest.protocol === 'rtmp') {
            // RTMP format: rtmp://IP:PORT/live/PASSWORD
            return `rtmp://${ip}:${ingest.port}/live/${ingest.stream_key || 'stream'}`;
        } else if (ingest.protocol === 'srt') {
            // SRT format: srt://IP:PORT?streamid=publish:KEY/stream
            // Encryption disabled (Option A).
            // We use the key in the path for "security by obscurity" routing.
            const password = ingest.stream_key || 'stream';
            // Encode the streamid value properly
            const streamID = `publish:${password}/stream`;

            return `srt://${ip}:${ingest.port}?streamid=${encodeURIComponent(streamID)}`;
        } else if (ingest.protocol === 'srtla') {
            // SRTLA format: srtla://IP:PORT?passphrase=PASSWORD
            // go-irl uses standard SRT encryption via -passphrase flag
            let url = `srtla://${ip}:${ingest.port}`;
            if (ingest.stream_key) {
                url += `?passphrase=${encodeURIComponent(ingest.stream_key)}`;
            }
            return url;
        }
        return '';
    };

    const handleAddSource = async (ingest: Ingest, sceneName: string) => {
        if (!sceneName) return;
        const sourceName = `${ingest.name}_MEDIA_SOURCE`;

        // Determine input format - only needed for raw streams, not RTSP
        const inputFormat = '';

        try {
            await api.post('/obs/source', {
                sceneName,
                sourceName,
                protocol: ingest.protocol,
                url: getOBSMediaSourceURL(ingest),
                inputFormat, // Pass mpegts for SRTLA UDP streams
            });
            // Success
            setAddedSources(prev => ({ ...prev, [`${ingest.id}-${sceneName}`]: true }));
        } catch (err: any) {
            // Check if error message contains "already exists"
            if (err.response?.data?.includes && err.response.data.includes("already exists")) {
                setAddedSources(prev => ({ ...prev, [`${ingest.id}-${sceneName}`]: true }));
            } else {
                console.error("Failed to add source", err);
                alert("Failed to add source to OBS. Ensure OBS is connected.");
            }
        }
    };

    const getOBSMediaSourceURL = (ingest: Ingest) => {
        if (ingest.protocol === 'srtla') {
            // SRTLA uses MediaMTX sidecar for RTSP egress
            const passphrase = ingest.stream_key || 'live';
            return `rtsp://127.0.0.1:${ingest.rtsp_port}/${passphrase}/stream`;
        }

        // MediaMTX SRT/RTMP egress via RTSP
        if (ingest.protocol === 'rtmp') {
            const key = ingest.stream_key || 'stream';
            return `rtsp://127.0.0.1:${ingest.output_port}/live/${key}`;
        }

        // SRT uses path from streamid: 'publish:PASSWORD/stream' or 'publish:stream'
        // MediaMTX path matches this structure
        const password = ingest.stream_key || '';
        const streamPath = password ? `${password}/stream` : 'stream';
        return `rtsp://127.0.0.1:${ingest.output_port}/${streamPath}`;
    };

    // Shareable URL for external users to pull the stream
    const getShareableURL = (ingest: Ingest) => {
        const ip = serverIP;

        if (ingest.protocol === 'srtla') {
            // SRTLA uses MediaMTX sidecar for RTSP egress (public)
            const passphrase = ingest.stream_key || 'live';
            return `rtsp://${ip}:${ingest.rtsp_port}/${passphrase}/stream`;
        }

        // MediaMTX SRT/RTMP egress via RTSP (public)
        if (ingest.protocol === 'rtmp') {
            const key = ingest.stream_key || 'stream';
            return `rtsp://${ip}:${ingest.output_port}/live/${key}`;
        }

        // SRT egress
        const password = ingest.stream_key || '';
        const streamPath = password ? `${password}/stream` : 'stream';
        return `rtsp://${ip}:${ingest.output_port}/${streamPath}`;
    };

    // Get correct label for OBS Media Source based on protocol
    const getOBSSourceLabel = (_protocol: string) => {
        // All protocols use RTSP egress via MediaMTX
        return 'OBS Media Source (RTSP)';
    };

    const copyToClipboard = (text: string) => {
        if (navigator.clipboard && window.isSecureContext) {
            navigator.clipboard.writeText(text);
            alert(`Copied: ${text}`);
        } else {
            // Fallback for non-secure context
            const textArea = document.createElement("textarea");
            textArea.value = text;
            textArea.style.position = "fixed";
            textArea.style.left = "-9999px";
            document.body.appendChild(textArea);
            textArea.focus();
            textArea.select();
            try {
                document.execCommand('copy');
                alert(`Copied: ${text}`);
            } catch (err) {
                console.error('Fallback: Oops, unable to copy', err);
            }
            document.body.removeChild(textArea);
        }
    };

    return (
        <div className="space-y-6">
            <div className="flex justify-between items-center">
                <h2 className="text-2xl font-bold dark:text-white text-dark-900">
                    Ingest Management
                </h2>
                <Button variant="secondary" onClick={fetchIngests} size="sm">
                    Refresh
                </Button>
            </div>

            <div className={`grid grid-cols-1 ${!ingestsLocked ? 'lg:grid-cols-4' : ''} gap-6`}>
                {/* Add New Ingest - hidden when locked */}
                {!ingestsLocked && (
                    <Card className="lg:col-span-1 p-6 h-fit">
                        <h3 className="text-lg font-semibold mb-4">Add New Ingest</h3>
                        <form onSubmit={handleSubmit} className="space-y-4">
                            <div>
                                <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-1">Name</label>
                                <Input
                                    value={formData.name}
                                    onChange={e => setFormData({ ...formData, name: e.target.value })}
                                    placeholder="My Private Ingest"
                                    required
                                />
                            </div>
                            <div>
                                <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-1">Protocol</label>
                                <select
                                    className="w-full dark:bg-dark-900/50 bg-light-200/50 border dark:border-dark-600 border-light-300 rounded-lg px-3 py-2 dark:text-white text-dark-900 focus:outline-none focus:ring-2 focus:ring-accent-red"
                                    value={formData.protocol}
                                    onChange={e => setFormData({ ...formData, protocol: e.target.value })}
                                >
                                    <option value="srtla">SRTLA (UDP/Multipath)</option>
                                    <option value="rtmp">RTMP</option>
                                    <option value="srt">SRT (Standard)</option>
                                </select>
                            </div>
                            <div>
                                <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-1">
                                    Password <span className="text-red-400">*</span>
                                </label>
                                <div className="flex gap-2">
                                    <Input
                                        value={formData.password}
                                        onChange={e => setFormData({ ...formData, password: e.target.value })}
                                        placeholder="Enter password"
                                        required
                                        minLength={10}
                                        className="flex-1"
                                    />
                                    <Button
                                        type="button"
                                        variant="secondary"
                                        size="sm"
                                        onClick={generatePassword}
                                        className="whitespace-nowrap"
                                    >
                                        Generate
                                    </Button>
                                </div>
                                <p className="text-xs dark:text-dark-400 text-dark-600 mt-1">
                                    {formData.protocol === 'srt'
                                        ? 'Min 10 characters. Used for path-based authentication (no encryption).'
                                        : 'Min 10 characters. Used for authentication.'}
                                </p>
                            </div>
                            <Button type="submit" disabled={isLoading || formData.password.length < 10} className="w-full">
                                {isLoading ? 'Creating...' : 'Create Ingest'}
                            </Button>

                        </form>
                    </Card>
                )}

                {/* List */}
                <Card className={!ingestsLocked ? 'lg:col-span-3' : 'lg:col-span-4'}>
                    <h2 className="text-xl font-bold dark:text-white text-dark-900 mb-6 flex items-center gap-2">
                        <svg className="w-5 h-5 text-accent-cyan" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                        </svg>
                        Active Ingests
                    </h2>

                    {/* Mobile Card View (md:hidden) */}
                    <div className="md:hidden space-y-4">
                        {ingests.length === 0 && (
                            <div className="p-6 text-center dark:text-dark-400 text-dark-600 italic dark:bg-dark-800/20 bg-light-200/50 rounded-lg border dark:border-dark-700/50 border-light-300">
                                No ingest servers configured.
                            </div>
                        )}
                        {ingests.map(ingest => (
                            <div key={ingest.id} className="dark:bg-dark-800/40 bg-light-200/50 border dark:border-dark-700/50 border-light-300 rounded-lg p-4 space-y-4">
                                <div className="flex justify-between items-start">
                                    <div className="flex-1">
                                        {editingIngestId === ingest.id ? (
                                            <div className="flex items-center gap-2">
                                                <Input
                                                    value={editingName}
                                                    onChange={e => setEditingName(e.target.value)}
                                                    className="text-lg font-bold min-w-[200px]"
                                                    autoFocus
                                                    onKeyDown={e => {
                                                        if (e.key === 'Enter') handleSaveRename();
                                                        if (e.key === 'Escape') handleCancelEdit();
                                                    }}
                                                />
                                                <button onClick={handleSaveRename} className="text-green-400 hover:text-green-300" title="Save">
                                                    <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" /></svg>
                                                </button>
                                                <button onClick={handleCancelEdit} className="text-red-400 hover:text-red-300" title="Cancel">
                                                    <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
                                                </button>
                                            </div>
                                        ) : (
                                            <div className="flex items-center gap-2">
                                                <h3 className="font-bold dark:text-white text-dark-900 text-lg">{ingest.name}</h3>
                                                <button onClick={() => handleStartEdit(ingest)} className="text-gray-400 hover:text-white" title="Edit name">
                                                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" /></svg>
                                                </button>
                                            </div>
                                        )}
                                        <div className="mt-1">
                                            <IngestStatus ingest={ingest} />
                                        </div>
                                    </div>
                                    <div className="flex gap-2">
                                        {!ingest.is_running ? (
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="text-green-400 bg-green-500/10 hover:bg-green-500/20"
                                                onClick={async () => {
                                                    try {
                                                        await api.post(`/ingests/${ingest.id}/start`);
                                                        fetchIngests();
                                                    } catch (e) { console.error(e); }
                                                }}
                                            >
                                                Start
                                            </Button>
                                        ) : (
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="text-yellow-400 bg-yellow-500/10 hover:bg-yellow-500/20"
                                                onClick={async () => {
                                                    try {
                                                        await api.post(`/ingests/${ingest.id}/stop`);
                                                        setTimeout(fetchIngests, 500);
                                                    } catch (e) { console.error(e); }
                                                }}
                                            >
                                                Stop
                                            </Button>
                                        )}
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            className="text-red-400 bg-red-500/10 hover:bg-red-500/20"
                                            onClick={() => handleDelete(ingest.id)}
                                        >
                                            Delete
                                        </Button>
                                    </div>
                                </div>

                                {/* Connections Info Mobile */}
                                <div className="space-y-3 pt-3 border-t dark:border-dark-700 border-light-300">
                                    {/* OBS Integration Mobile */}
                                    <div className="bg-blue-500/5 p-3 rounded border border-blue-500/20">
                                        <div className="text-xs text-blue-400 uppercase font-bold tracking-wider mb-2">Add to OBS</div>
                                        <div className="flex gap-2">
                                            <select
                                                className="flex-1 bg-dark-900/50 border border-blue-500/30 rounded px-2 py-1 text-sm text-white focus:outline-none"
                                                value={selectedScenes[ingest.id] || ''}
                                                onChange={(e) => setSelectedScenes(prev => ({ ...prev, [ingest.id]: e.target.value }))}
                                            >
                                                <option value="">Select Scene...</option>
                                                {obsScenes.map(s => <option key={s} value={s}>{s}</option>)}
                                            </select>
                                            <Button
                                                size="sm"
                                                className={`whitespace-nowrap ${addedSources[`${ingest.id}-${selectedScenes[ingest.id]}`] ? 'bg-green-600 hover:bg-green-700' : 'bg-accent-blue hover:bg-blue-600'}`}
                                                onClick={() => {
                                                    const scene = selectedScenes[ingest.id];
                                                    if (scene) handleAddSource(ingest, scene);
                                                }}
                                                disabled={!selectedScenes[ingest.id] || !!addedSources[`${ingest.id}-${selectedScenes[ingest.id]}`]}
                                            >
                                                {addedSources[`${ingest.id}-${selectedScenes[ingest.id]}`] ? 'Added' : 'Add'}
                                            </Button>
                                        </div>
                                    </div>
                                    <div>
                                        <div className="text-xs text-accent-red uppercase font-bold tracking-wider mb-1">Input (Public)</div>
                                        <div className="flex items-center gap-2 bg-red-500/10 p-2 rounded border border-red-500/20">
                                            <span className="font-mono text-xs text-accent-red break-all select-all">
                                                {getConnectionString(ingest)}
                                            </span>
                                            <button onClick={() => copyToClipboard(getConnectionString(ingest))} className="text-gray-500 hover:text-white shrink-0">
                                                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 01-2-2V5a2 2 0 012-2h4.586" /></svg>
                                            </button>
                                        </div>
                                    </div>

                                    {/* OBS Media Source Mobile */}
                                    {ingest.output_port > 0 && (
                                        <div>
                                            <div className="text-xs text-accent-cyan uppercase font-bold tracking-wider mb-1">{getOBSSourceLabel(ingest.protocol)}</div>
                                            <div className="flex items-center gap-2 bg-cyan-500/10 p-2 rounded border border-cyan-500/20">
                                                <span className="font-mono text-xs text-accent-cyan break-all select-all">
                                                    {getOBSMediaSourceURL(ingest)}
                                                </span>
                                                <button onClick={() => copyToClipboard(getOBSMediaSourceURL(ingest))} className="text-gray-500 hover:text-white shrink-0">
                                                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 01-2-2V5a2 2 0 012-2h4.586" /></svg>
                                                </button>
                                            </div>
                                        </div>
                                    )}

                                    {/* Shareable OBS Source Mobile */}
                                    {ingest.output_port > 0 && (
                                        <div>
                                            <div className="text-xs text-green-400 uppercase font-bold tracking-wider mb-1">Shareable OBS Source</div>
                                            <div className="flex items-center gap-2 bg-green-500/10 p-2 rounded border border-green-500/20">
                                                <span className="font-mono text-xs text-green-300 break-all select-all">
                                                    {getShareableURL(ingest)}
                                                </span>
                                                <button onClick={() => copyToClipboard(getShareableURL(ingest))} className="text-gray-500 hover:text-white shrink-0">
                                                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 01-2-2V5a2 2 0 012-2h4.586" /></svg>
                                                </button>
                                            </div>
                                        </div>
                                    )}


                                </div>
                            </div>
                        ))}
                    </div>

                    {/* Desktop Table View (hidden on mobile) */}
                    <div className="hidden md:block overflow-x-auto rounded-lg border dark:border-dark-700/50 border-light-300">
                        <table className="w-full text-left text-sm">
                            <thead className="dark:bg-dark-800/80 bg-light-200 dark:text-dark-300 text-dark-700 uppercase tracking-wider font-semibold">
                                <tr>
                                    <th className="px-6 py-4">Name</th>
                                    <th className="px-6 py-4">Connections</th>
                                    <th className="px-6 py-4">Status</th>
                                    <th className="px-6 py-4">OBS Easy Add</th>
                                    <th className="px-6 py-4 text-right">Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y dark:divide-dark-700/50 divide-light-300 dark:bg-dark-900/20 bg-white/50">
                                {ingests.length === 0 && (
                                    <tr>
                                        <td colSpan={4} className="px-6 py-8 text-center dark:text-dark-400 text-dark-600 italic">
                                            No ingest servers configured. Add one to get started.
                                        </td>
                                    </tr>
                                )}
                                {ingests.map(ingest => (
                                    <tr key={ingest.id} className="dark:hover:bg-dark-800/30 hover:bg-light-200 transition-colors">
                                        <td className="px-6 py-4 font-medium dark:text-white text-dark-900">
                                            {editingIngestId === ingest.id ? (
                                                <div className="flex items-center gap-2">
                                                    <Input
                                                        value={editingName}
                                                        onChange={e => setEditingName(e.target.value)}
                                                        className="font-medium min-w-[200px]"
                                                        autoFocus
                                                        onKeyDown={e => {
                                                            if (e.key === 'Enter') handleSaveRename();
                                                            if (e.key === 'Escape') handleCancelEdit();
                                                        }}
                                                    />
                                                    <button onClick={handleSaveRename} className="text-green-400 hover:text-green-300" title="Save">
                                                        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" /></svg>
                                                    </button>
                                                    <button onClick={handleCancelEdit} className="text-red-400 hover:text-red-300" title="Cancel">
                                                        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
                                                    </button>
                                                </div>
                                            ) : (
                                                <div className="flex items-center gap-2">
                                                    <span>{ingest.name}</span>
                                                    <button onClick={() => handleStartEdit(ingest)} className="text-gray-400 hover:text-white" title="Edit name">
                                                        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" /></svg>
                                                    </button>
                                                </div>
                                            )}
                                        </td>
                                        <td className="px-6 py-4">
                                            <div className="flex flex-col space-y-3">
                                                {/* Input (Public) */}
                                                <div>
                                                    <div className="text-xs text-accent-red uppercase font-bold tracking-wider mb-0.5">Input (Public)</div>
                                                    <div className="flex items-center gap-2">
                                                        <span className="font-mono text-accent-red bg-red-500/10 px-1.5 py-0.5 rounded border border-red-500/20">
                                                            {getConnectionString(ingest)}
                                                        </span>
                                                        <button
                                                            onClick={() => copyToClipboard(getConnectionString(ingest))}
                                                            className="text-gray-500 hover:text-white"
                                                            title="Copy Connection String"
                                                        >
                                                            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 01-2-2V5a2 2 0 012-2h4.586" /></svg>
                                                        </button>
                                                    </div>
                                                </div>

                                                {/* Output (OBS Media Source - Local) */}
                                                {ingest.output_port > 0 && (
                                                    <div>
                                                        <div className="text-xs text-accent-cyan uppercase font-bold tracking-wider mb-0.5">{getOBSSourceLabel(ingest.protocol)}</div>
                                                        <div className="flex items-center gap-2">
                                                            <div className="font-mono text-accent-cyan bg-cyan-500/10 px-1.5 py-0.5 rounded w-fit border border-cyan-500/20">
                                                                {getOBSMediaSourceURL(ingest)}
                                                            </div>
                                                            <button
                                                                onClick={() => copyToClipboard(getOBSMediaSourceURL(ingest))}
                                                                className="text-gray-500 hover:text-white"
                                                                title="Copy Output"
                                                            >
                                                                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 01-2-2V5a2 2 0 012-2h4.586" /></svg>
                                                            </button>
                                                        </div>
                                                    </div>
                                                )}

                                                {/* Shareable OBS Source URL (for external users) */}
                                                {ingest.output_port > 0 && (
                                                    <div className="mt-2">
                                                        <div className="text-xs text-green-400 uppercase font-bold tracking-wider mb-0.5">Shareable OBS Source</div>
                                                        <div className="flex items-center gap-2">
                                                            <div className="font-mono text-green-300 bg-green-500/10 px-1.5 py-0.5 rounded w-fit border border-green-500/20">
                                                                {getShareableURL(ingest)}
                                                            </div>
                                                            <button
                                                                onClick={() => copyToClipboard(getShareableURL(ingest))}
                                                                className="text-gray-500 hover:text-white"
                                                                title="Copy Shareable URL"
                                                            >
                                                                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 01-2-2V5a2 2 0 012-2h4.586" /></svg>
                                                            </button>
                                                        </div>
                                                    </div>
                                                )}



                                            </div>
                                        </td>
                                        <td className="px-6 py-4">
                                            <IngestStatus ingest={ingest} />
                                        </td>
                                        <td className="px-6 py-4">
                                            <div className="flex items-center gap-2">
                                                <select
                                                    className="bg-dark-900/50 border border-dark-600 rounded px-2 py-1 text-sm text-white w-32 focus:outline-none focus:border-accent-blue"
                                                    value={selectedScenes[ingest.id] || ''}
                                                    onChange={(e) => setSelectedScenes(prev => ({ ...prev, [ingest.id]: e.target.value }))}
                                                >
                                                    <option value="">Target Scene</option>
                                                    {obsScenes.map(s => <option key={s} value={s}>{s}</option>)}
                                                </select>
                                                <Button
                                                    size="sm"
                                                    className={`border-none ${addedSources[`${ingest.id}-${selectedScenes[ingest.id]}`] ? 'bg-green-600 hover:bg-green-700 text-white' : 'bg-accent-blue hover:bg-blue-600 text-white'}`}
                                                    onClick={() => {
                                                        const scene = selectedScenes[ingest.id];
                                                        if (scene) handleAddSource(ingest, scene);
                                                    }}
                                                    disabled={!selectedScenes[ingest.id] || !!addedSources[`${ingest.id}-${selectedScenes[ingest.id]}`]}
                                                >
                                                    {addedSources[`${ingest.id}-${selectedScenes[ingest.id]}`] ? 'Added' : 'Add'}
                                                </Button>
                                            </div>
                                        </td>
                                        <td className="px-6 py-4 text-right space-x-2">
                                            {!ingest.is_running ? (
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    className="text-green-400 hover:text-green-300 hover:bg-green-500/10"
                                                    onClick={async () => {
                                                        try {
                                                            await api.post(`/ingests/${ingest.id}/start`);
                                                            fetchIngests();
                                                        } catch (e) { console.error(e); }
                                                    }}
                                                >
                                                    Start
                                                </Button>
                                            ) : (
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    className="text-yellow-400 hover:text-yellow-300 hover:bg-yellow-500/10"
                                                    onClick={async () => {
                                                        try {
                                                            await api.post(`/ingests/${ingest.id}/stop`);
                                                            setTimeout(fetchIngests, 500);
                                                        } catch (e) { console.error(e); }
                                                    }}
                                                >
                                                    Stop
                                                </Button>
                                            )}

                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="text-red-400 hover:text-red-300 hover:bg-red-500/10"
                                                onClick={() => handleDelete(ingest.id)}
                                            >
                                                Delete
                                            </Button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </Card>
            </div>
        </div>
    );
};

export default SettingsIngests;
