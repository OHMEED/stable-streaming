import React, { useEffect, useState, useRef } from 'react';
import api from '../lib/api';
import { Card } from '../components/ui/Card';

interface Ingest {
    id: number;
    name: string;
}

const SettingsLogs: React.FC = () => {
    const [ingests, setIngests] = useState<Ingest[]>([]);
    const [selectedLogId, setSelectedLogId] = useState<string>('app');
    const [logContent, setLogContent] = useState<string>('');
    const [isAutoScroll, setIsAutoScroll] = useState(true);
    const scrollRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        fetchIngests();
    }, []);

    const fetchIngests = async () => {
        try {
            const res = await api.get('/ingests');
            setIngests(res.data || []);
            // Keep 'app' as default, don't auto-switch to first ingest
        } catch (err) {
            console.error(err);
        }
    };

    useEffect(() => {
        if (!selectedLogId) return;

        const fetchLogs = async () => {
            try {
                let url = '';
                if (selectedLogId.startsWith('ingest_')) {
                    const id = selectedLogId.split('_')[1];
                    url = `/ingests/${id}/logs`;
                } else if (selectedLogId === 'multistream') {
                    url = `/multistream/logs`;
                } else {
                    url = `/logs/app`;
                }

                const res = await api.get(url);
                setLogContent(res.data.content || '');
            } catch (err) {
                console.error(err);
                setLogContent('Failed to fetch logs or log file does not exist yet.');
            }
        };

        fetchLogs();
        const interval = setInterval(fetchLogs, 2000);
        return () => clearInterval(interval);
    }, [selectedLogId]);

    useEffect(() => {
        if (isAutoScroll && scrollRef.current) {
            scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
        }
    }, [logContent, isAutoScroll]);

    return (
        <div className="space-y-6 h-[calc(100vh-140px)] flex flex-col">
            <div className="flex items-center justify-between">
                <h1 className="text-3xl font-bold dark:text-white text-dark-900 mb-2">Logs</h1>
                <select
                    className="dark:bg-dark-800 bg-light-200 dark:text-white text-dark-900 border dark:border-dark-700 border-light-300 rounded px-3 py-1 text-sm focus:outline-none focus:border-accent-red"
                    value={selectedLogId}
                    onChange={(e) => setSelectedLogId(e.target.value)}
                >
                    <option value="app">Application Log</option>
                    <option value="multistream">Multi-Stream (RTMP)</option>
                    {ingests.map(ingest => (
                        <option key={ingest.id} value={`ingest_${ingest.id}`}>{ingest.name} Log</option>
                    ))}
                </select>
            </div>

            <Card className="flex-1 flex flex-col p-0 overflow-hidden dark:border-dark-700/50 border-light-300/50">
                <div className="flex items-center justify-end p-4 border-b dark:border-dark-700 border-light-300 dark:bg-dark-800/30 bg-light-200/50">
                    <div className="flex items-center gap-4">
                        <span className="text-xs dark:text-dark-400 text-dark-600 font-mono">
                            {selectedLogId === 'app' ? 'APPLICATION.LOG' : selectedLogId.replace('_', ' ').toUpperCase() + '.LOG'}
                        </span>
                        <label className="flex items-center gap-2 text-xs dark:text-dark-300 text-dark-700 cursor-pointer select-none">
                            <input
                                type="checkbox"
                                checked={isAutoScroll}
                                onChange={(e) => setIsAutoScroll(e.target.checked)}
                                className="rounded dark:border-dark-600 border-light-400 dark:bg-dark-800 bg-light-100 text-accent-red focus:ring-accent-red/50"
                            />
                            Auto-scroll
                        </label>
                    </div>
                </div>

                <div
                    ref={scrollRef}
                    className="flex-1 p-6 overflow-y-auto font-mono text-xs md:text-sm space-y-1 dark:text-dark-200 text-dark-900 whitespace-pre-wrap dark:bg-dark-900/50 bg-light-100"
                >
                    {logContent || <span className="dark:text-dark-500 text-dark-500 italic">Waiting for logs...</span>}
                    {/* Fake cursor at end */}
                    <span className="animate-pulse inline-block w-2 h-4 bg-accent-red align-middle ml-1"></span>
                </div>
            </Card >
        </div >
    );
};

export default SettingsLogs;
