import React, { useState } from 'react';
import api from '../lib/api';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';

const System: React.FC = () => {
    const [isRestarting, setIsRestarting] = useState(false);

    const handleRestartService = async () => {
        if (!confirm("Are you sure you want to restart the backend service? This will interrupt active streams.")) return;
        setIsRestarting(true);
        try {
            await api.post('/system/restart');
            alert('Service restarting...');
        } catch (e) {
            console.error(e);
            alert('Triggered restart (check logs)');
        } finally {
            setTimeout(() => setIsRestarting(false), 2000);
        }
    };

    return (
        <div className="space-y-6">
            <h1 className="text-3xl font-bold dark:text-white text-dark-900 mb-8">System Settings</h1>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <Card>
                    <h2 className="text-xl font-bold dark:text-white text-dark-900 mb-4 flex items-center gap-2">
                        <svg className="w-5 h-5 text-accent-red" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                        </svg>
                        Service Control
                    </h2>
                    <p className="dark:text-dark-300 text-dark-600 mb-6">
                        Manage the core backend service. Restarting will disconnect all active ingests and control connections.
                    </p>
                    <Button
                        onClick={handleRestartService}
                        variant="danger"
                        isLoading={isRestarting}
                        className="w-full sm:w-auto"
                    >
                        Restart Backend Service
                    </Button>
                </Card>

                <Card>
                    <h2 className="text-xl font-bold dark:text-white text-dark-900 mb-4 flex items-center gap-2">
                        <svg className="w-5 h-5 text-accent-cyan" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        </svg>
                        Maintenance
                    </h2>
                    <p className="dark:text-dark-300 text-dark-600 mb-6">
                        System maintenance tools and cleanup operations.
                    </p>
                    <Button
                        variant="secondary"
                        disabled
                        className="w-full sm:w-auto"
                    >
                        Clear Cache (Coming Soon)
                    </Button>
                </Card>
            </div>

            <Card className="flex flex-col h-[500px]">
                <h2 className="text-xl font-bold dark:text-white text-dark-900 mb-4 flex items-center gap-2">
                    <svg className="w-5 h-5 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                    </svg>
                    System Logs
                </h2>
                <div className="flex-1 dark:bg-dark-900/70 bg-dark-900/90 rounded-lg border dark:border-dark-700 border-dark-600 p-4 overflow-y-auto font-mono text-xs space-y-1 scrollbar-thin scrollbar-thumb-gray-700 scrollbar-track-transparent">
                    <div className="text-green-400">[INFO] System started successfully</div>
                    <div className="text-accent-cyan">[INFO] Connected to SQLite Database</div>
                    <div className="text-dark-400">[DEBUG] Loading ingest configuration...</div>
                    <div className="text-yellow-400">[WARN] SRT port 9000 is reserved</div>
                    <div className="text-green-400">[INFO] Ingest Manager initialized</div>
                    <div className="text-accent-red">[SYSTEM] WebSocket server listening on :8080</div>
                    <div className="text-dark-400">... waiting for new logs ...</div>
                </div>
            </Card>
        </div>
    );
};

export default System;
