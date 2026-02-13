import React, { useEffect, useState } from 'react';
import api from '../lib/api';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { IngestStatus } from '../components/IngestStatus';

interface Ingest {
    id: number;
    name: string;
    protocol: string;
    port: number;
    enabled: boolean;
    is_running: boolean;
}

const Dashboard: React.FC = () => {
    const [ingests, setIngests] = useState<Ingest[]>([]);
    const [streamStatus, setStreamStatus] = useState<'off' | 'live' | 'starting'>('off');
    const [recordStatus, setRecordStatus] = useState<'off' | 'on' | 'starting'>('off');
    const [scenes, setScenes] = useState<string[]>([]);
    const [currentScene, setCurrentScene] = useState('Starting Soon');
    const [isLoadingStream, setIsLoadingStream] = useState(false);
    const [isLoadingRecord, setIsLoadingRecord] = useState(false);
    const [previewImage, setPreviewImage] = useState<string | null>(null);
    const [isObsConnected, setIsObsConnected] = useState(false);

    const [isPreviewLoading, setIsPreviewLoading] = useState(false);

    useEffect(() => {
        fetchIngests();
        fetchScenes();
        fetchStatus();
        fetchPreview();

        const statusInterval = setInterval(fetchStatus, 3000);
        const previewInterval = setInterval(fetchPreview, 500);

        return () => {
            clearInterval(statusInterval);
            clearInterval(previewInterval);
        };
    }, []);

    const fetchStatus = async () => {
        try {
            const res = await api.get('/obs/status');
            setIsObsConnected(res.data.connected);
            setStreamStatus(res.data.streaming ? 'live' : 'off');
            setRecordStatus(res.data.recording ? 'on' : 'off');

            if (res.data.currentScene) {
                setCurrentScene(res.data.currentScene);
            }

            // If scenes failed to load initially, try again once connected
            if (res.data.connected && scenes.length === 0) {
                fetchScenes();
            }
        } catch (err) {
            console.error("Failed to fetch OBS status", err);
            setIsObsConnected(false);
        }
    };

    const fetchPreview = async () => {
        if (isPreviewLoading) return;
        setIsPreviewLoading(true);
        try {
            const res = await api.get('/obs/preview');
            if (res.data && res.data.image) {
                const img = res.data.image;
                if (img.startsWith('data:')) {
                    setPreviewImage(img);
                } else {
                    setPreviewImage(`data:image/jpeg;base64,${img}`);
                }
            }
        } catch (err) {
            console.error("Failed to fetch preview", err);
        } finally {
            setIsPreviewLoading(false);
        }
    };

    const fetchScenes = async () => {
        try {
            const res = await api.get('/obs/scenes');
            if (res.data && Array.isArray(res.data)) {
                setScenes(res.data);
            }
        } catch (err) {
            console.error("Failed to fetch scenes", err);
        }
    };

    const fetchIngests = async () => {
        try {
            const res = await api.get('/ingests');
            setIngests(res.data || []);
        } catch (err) {
            console.error(err);
        }
    };

    const toggleStream = async () => {
        setIsLoadingStream(true);
        try {
            await api.post('/obs/stream/toggle');
            await fetchStatus();
        } catch (err) {
            console.error("Failed to toggle stream", err);
        } finally {
            setIsLoadingStream(false);
        }
    };

    const toggleRecord = async () => {
        setIsLoadingRecord(true);
        try {
            await api.post('/obs/record/toggle');
            await fetchStatus();
        } catch (err) {
            console.error("Failed to toggle record", err);
        } finally {
            setIsLoadingRecord(false);
        }
    };

    const switchScene = async (sceneName: string) => {
        try {
            await api.post('/obs/scene', { sceneName: sceneName });
            setCurrentScene(sceneName);
        } catch (err) {
            console.error(err);
        }
    };

    return (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
            {/* Stream Control & Status */}
            <div className="lg:col-span-2 space-y-6">
                <Card className="relative overflow-hidden mb-6">
                    <div className="flex flex-col md:flex-row items-center justify-center gap-4 relative z-10 py-2">
                        <Button
                            onClick={toggleStream}
                            variant={streamStatus === 'live' ? 'primary' : 'secondary'}
                            size="lg"
                            isLoading={isLoadingStream}
                            className={`min-w-[200px] shadow-xl transition-all duration-500 font-bold ${streamStatus === 'live'
                                ? 'bg-pulse-green border-green-400 text-white'
                                : 'bg-transparent border-green-500/50 text-green-600 dark:text-green-500 hover:bg-green-500/10'
                                }`}
                        >
                            {streamStatus === 'live' ? 'STOP STREAM' : 'GO LIVE'}
                        </Button>

                        <Button
                            onClick={toggleRecord}
                            variant={recordStatus === 'on' ? 'danger' : 'secondary'}
                            size="lg"
                            isLoading={isLoadingRecord}
                            className={`min-w-[200px] shadow-xl transition-all duration-500 font-bold ${recordStatus === 'on'
                                ? 'bg-pulse-red border-red-400 text-white'
                                : 'bg-transparent border-red-500/50 text-red-600 dark:text-red-500 hover:bg-red-500/10'
                                }`}
                        >
                            {recordStatus === 'on' ? 'STOP RECORDING' : 'START RECORDING'}
                        </Button>
                    </div>
                </Card>

                {/* OBS Preview */}
                <Card>
                    <div className="flex items-center justify-between mb-4">
                        <div className="flex items-center gap-3">
                            <h2 className="text-xl font-bold dark:text-white text-dark-900">OBS Preview</h2>
                            {isObsConnected && (
                                <div className="flex gap-2">
                                    {streamStatus === 'live' && <Badge variant="error" size="sm" pulse>LIVE</Badge>}
                                    {recordStatus === 'on' && <Badge variant="error" size="sm" pulse>REC</Badge>}
                                </div>
                            )}
                        </div>
                        <Badge variant={isObsConnected ? "info" : "error"} size="sm">
                            {isObsConnected ? "WebSocket Connected" : "WebSocket Disconnected"}
                        </Badge>
                    </div>
                    <div className="aspect-video dark:bg-black/50 bg-dark-900/80 rounded-lg border dark:border-dark-700 border-light-300 flex items-center justify-center relative overflow-hidden group">
                        {(isObsConnected && previewImage) ? (
                            <img
                                key={previewImage.substring(0, 100)} // Use a slice of the string as key to force refresh
                                src={previewImage}
                                alt="OBS Preview"
                                className="w-full h-full object-cover"
                            />
                        ) : (
                            <div className="flex flex-col items-center gap-4 dark:text-dark-400 text-light-500">
                                <svg className="w-12 h-12 lg:w-16 lg:h-16 animate-pulse opacity-20" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                                </svg>
                                <span className="font-mono text-sm uppercase tracking-widest">
                                    {!isObsConnected ? "WebSocket Offline" : "Signal Search..."}
                                </span>
                            </div>
                        )}
                        <div className="absolute inset-0 bg-gradient-to-t from-black/80 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-end p-4">
                            <p className="text-sm text-dark-200">Live monitor from OBS Program output.</p>
                        </div>
                    </div>
                </Card>
            </div>

            {/* Sidebar Widgets */}
            <div className="space-y-6">
                {/* Scene Switcher */}
                <Card>
                    <h2 className="text-lg font-bold dark:text-white text-dark-900 mb-4 flex items-center gap-2">
                        <svg className="w-5 h-5 text-accent-red" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
                        </svg>
                        Scenes
                    </h2>
                    <div className="grid grid-cols-1 gap-2">
                        {scenes.length > 0 ? scenes.map(scene => (
                            <button
                                key={scene}
                                onClick={() => switchScene(scene)}
                                className={`
                                    w-full text-left px-4 py-3 rounded-lg font-medium transition-all duration-200 border
                                    ${currentScene === scene
                                        ? 'bg-accent-red border-accent-red text-white shadow-lg shadow-accent-red/20'
                                        : 'dark:bg-dark-800/50 bg-light-200/50 dark:border-dark-700/50 border-light-300/50 dark:text-dark-300 text-dark-600 dark:hover:bg-dark-700 hover:bg-light-300 dark:hover:text-white hover:text-dark-900 dark:hover:border-dark-600 hover:border-light-400'}
                                `}
                            >
                                {scene}
                            </button>
                        )) : (
                            <div className="text-center py-8 dark:text-dark-400 text-dark-500 text-sm">No scenes found</div>
                        )}
                    </div>
                </Card>

                {/* Ingest Status */}
                <Card>
                    <h2 className="text-lg font-bold dark:text-white text-dark-900 mb-4 flex items-center gap-2">
                        <svg className="w-5 h-5 text-accent-cyan" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                        </svg>
                        Ingest Status
                    </h2>
                    <div className="space-y-3">
                        {ingests.length === 0 && <span className="dark:text-dark-400 text-dark-500 text-sm italic">No ingests configured</span>}
                        {ingests.map(ingest => (
                            <div key={ingest.id} className="flex items-center justify-between p-3 dark:bg-dark-800/50 bg-light-200/50 rounded-lg border dark:border-dark-700/50 border-light-300/50 dark:hover:border-dark-600 hover:border-light-400 transition-colors">
                                <div className="flex flex-col">
                                    <span className="font-semibold dark:text-dark-200 text-dark-700">{ingest.name}</span>
                                    <span className="text-xs text-accent-red font-mono uppercase">{ingest.protocol} :{ingest.port}</span>
                                </div>
                                <IngestStatus ingest={ingest} />
                            </div>
                        ))}
                    </div>
                </Card>
            </div>
        </div>
    );
};

export default Dashboard;
