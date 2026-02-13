import React, { useEffect, useState, useRef } from 'react';
import api from '../lib/api';

interface IngestStatusProps {
    ingest: {
        id: number;
        protocol: string;
        is_running: boolean;
        enabled: boolean;
    };
}

export const IngestStatus: React.FC<IngestStatusProps> = ({ ingest }) => {
    const [bitrate, setBitrate] = useState<number>(0); // in Mbps
    const [status, setStatus] = useState<'offline' | 'ready' | 'receiving'>('offline');

    // Use refs for calculation state to avoid re-triggering effects
    const lastBytes = useRef<number>(0);
    const lastTime = useRef<number>(Date.now());

    // Use ref to keep track of active polling to avoid memory leaks
    const isMounted = useRef(true);

    useEffect(() => {
        isMounted.current = true;
        return () => { isMounted.current = false; };
    }, []);

    useEffect(() => {
        if (!ingest.is_running) {
            setStatus('offline');
            setBitrate(0);
            return;
        }

        // Initial state is at least ready if running
        setStatus(prev => prev === 'offline' ? 'ready' : prev);

        const fetchStats = async () => {
            try {
                const res = await api.get(`/ingests/${ingest.id}/stats`);
                if (!isMounted.current) return;

                const data = res.data;
                const now = Date.now();

                // Logic for SRTLA
                if (ingest.protocol === 'srtla') {
                    // stable-srtla usually returns { bitrate_kbps: 1234, ... }
                    let mbps = 0;
                    if (typeof data.bitrate_kbps === 'number') mbps = data.bitrate_kbps / 1000;
                    else if (typeof data.kbps === 'number') mbps = data.kbps / 1000;
                    else if (typeof data.bitrate === 'number') mbps = data.bitrate / 1000000; // Assuming bits/s if 'bitrate'

                    if (mbps > 0.01) {
                        setStatus('receiving');
                        setBitrate(mbps);
                    } else {
                        setStatus('ready');
                        setBitrate(0);
                    }
                }
                // Logic for MediaMTX (SRT/RTMP) - v3/paths/list response
                else if (ingest.protocol === 'srt' || ingest.protocol === 'rtmp') {
                    if (data.itemCount && data.itemCount > 0 && data.items && data.items.length > 0) {
                        // Assuming single path/stream per ingest port
                        const item = data.items[0];

                        // Check for ready state (MediaMTX v3 uses 'ready', older might use 'sourceReady')
                        if (item.ready) {
                            // Calculate bitrate from bytesReceived
                            const currentBytes = item.bytesReceived || 0;

                            // If we have history, calc rate
                            if (lastBytes.current > 0 && now > lastTime.current) {
                                const diffBytes = currentBytes - lastBytes.current;
                                const diffSec = (now - lastTime.current) / 1000;
                                if (diffSec > 0) {
                                    const bytesPerSec = diffBytes / diffSec;
                                    const mbps = (bytesPerSec * 8) / 1000000;
                                    setBitrate(mbps);
                                }
                            }

                            lastBytes.current = currentBytes;
                            lastTime.current = now;
                            setStatus('receiving');
                        } else {
                            setStatus('ready'); // Path exists but source not ready?
                        }
                    } else {
                        setStatus('ready'); // MediaMTX running but no paths (idle)
                    }
                }
            } catch (err) {
                // If stats fail but process running, assume Ready
                // console.error(`Failed to fetch stats for ${ingest.id}`, err);
            }
        };

        // Poll every 2 seconds
        const interval = setInterval(fetchStats, 2000);
        fetchStats(); // Initial call

        return () => clearInterval(interval);
    }, [ingest.id, ingest.is_running, ingest.protocol]); // Removed lastBytes/lastTime from deps

    if (!ingest.is_running) {
        return (
            <div className="flex items-center gap-2 text-red-500 bg-red-500/10 px-3 py-1 rounded-full border border-red-500/20">
                <div className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />
                <span className="text-xs font-bold uppercase">Not Running</span>
            </div>
        );
    }

    if (status === 'ready') {
        return (
            <div className="flex items-center gap-2 text-cyan-400 bg-cyan-500/10 px-3 py-1 rounded-full border border-cyan-500/20 shadow-[0_0_10px_rgba(34,211,238,0.1)]">
                <div className="w-2 h-2 rounded-full bg-cyan-400" />
                <span className="text-xs font-bold uppercase">Ready</span>
            </div>
        );
    }

    // Receiving
    return (
        <div className="flex items-center gap-2 text-green-400 bg-green-500/10 px-3 py-1 rounded-full border border-green-500/20 shadow-[0_0_10px_rgba(74,222,128,0.1)]">
            <div className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
            <span className="text-xs font-bold uppercase">
                Receiving {bitrate.toFixed(2)} Mbps
            </span>
        </div>
    );
};
