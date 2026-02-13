import React, { useState, useEffect } from 'react';
import { Button } from '../components/ui/Button';
import api from '../lib/api';

interface VersionInfo {
    current_version: string;
    latest_version: string;
    update_available: boolean;
}

const SettingsUpdate: React.FC = () => {
    const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
    const [loading, setLoading] = useState(true);
    const [updating, setUpdating] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [updateMessage, setUpdateMessage] = useState<string | null>(null);

    const fetchVersion = async () => {
        setLoading(true);
        setError(null);
        try {
            const res = await api.get('/system/version');
            setVersionInfo(res.data);
        } catch (err) {
            setError('Failed to check version');
            console.error(err);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        fetchVersion();
    }, []);

    const handleUpdate = async () => {
        if (!confirm('This will update the application and restart the service. You will be temporarily disconnected. Continue?')) {
            return;
        }

        setUpdating(true);
        setUpdateMessage('Starting update...');
        setError(null);

        try {
            await api.post('/system/update');
            setUpdateMessage('Update started. The service is restarting...');

            // Wait and then try to reconnect
            setTimeout(() => {
                setUpdateMessage('Waiting for service to restart...');
            }, 3000);

            // Poll for server to come back online
            let attempts = 0;
            const maxAttempts = 30;
            const pollInterval = setInterval(async () => {
                attempts++;
                try {
                    await api.get('/status');
                    clearInterval(pollInterval);
                    setUpdateMessage('Update complete! Refreshing...');
                    setTimeout(() => {
                        window.location.reload();
                    }, 1000);
                } catch {
                    if (attempts >= maxAttempts) {
                        clearInterval(pollInterval);
                        setUpdating(false);
                        setError('Update may have failed or is taking longer than expected. Please check the server status.');
                    }
                }
            }, 2000);
        } catch (err) {
            setUpdating(false);
            setError('Failed to start update');
            console.error(err);
        }
    };

    return (
        <div className="space-y-6">
            <h1 className="text-2xl font-bold dark:text-white text-dark-900">System Update</h1>

            <div className="glass rounded-2xl p-6 space-y-6">
                {/* Current Version */}
                <div className="flex items-center justify-between">
                    <div>
                        <h2 className="text-lg font-semibold dark:text-white text-dark-900">Current Version</h2>
                        <p className="dark:text-dark-300 text-dark-600">
                            {loading ? 'Loading...' : versionInfo?.current_version || 'Unknown'}
                        </p>
                    </div>
                    <Button
                        onClick={fetchVersion}
                        variant="ghost"
                        size="sm"
                        disabled={loading || updating}
                    >
                        {loading ? 'Checking...' : 'Check for Updates'}
                    </Button>
                </div>

                {/* Update Status */}
                {versionInfo && (
                    <div className={`p-4 rounded-xl ${versionInfo.update_available
                            ? 'bg-accent-cyan/10 border border-accent-cyan/30'
                            : 'bg-green-500/10 border border-green-500/30'
                        }`}>
                        {versionInfo.update_available ? (
                            <div className="flex items-center justify-between">
                                <div>
                                    <p className="font-medium text-accent-cyan">Update Available</p>
                                    <p className="text-sm dark:text-dark-300 text-dark-600">
                                        Latest: {versionInfo.latest_version}
                                    </p>
                                </div>
                                <Button
                                    onClick={handleUpdate}
                                    variant="primary"
                                    disabled={updating}
                                >
                                    {updating ? 'Updating...' : 'Update Now'}
                                </Button>
                            </div>
                        ) : (
                            <div className="flex items-center gap-2">
                                <svg className="w-5 h-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                                </svg>
                                <span className="text-green-500 font-medium">You're up to date!</span>
                            </div>
                        )}
                    </div>
                )}

                {/* Update Progress */}
                {updating && updateMessage && (
                    <div className="p-4 rounded-xl bg-accent-red/10 border border-accent-red/30">
                        <div className="flex items-center gap-3">
                            <div className="animate-spin rounded-full h-5 w-5 border-2 border-accent-red border-t-transparent"></div>
                            <span className="text-accent-red">{updateMessage}</span>
                        </div>
                    </div>
                )}

                {/* Error */}
                {error && (
                    <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/30">
                        <p className="text-red-500">{error}</p>
                    </div>
                )}

                {/* Info */}
                <div className="pt-4 border-t dark:border-dark-700 border-light-300">
                    <h3 className="font-medium dark:text-white text-dark-900 mb-2">About Updates</h3>
                    <ul className="text-sm dark:text-dark-400 text-dark-500 space-y-1">
                        <li>• Updates are pulled from GitHub (main branch)</li>
                        <li>• The service will restart during updates</li>
                        <li>• Your configuration and data will be preserved</li>
                        <li>• You can also update manually via SSH: <code className="px-1 py-0.5 rounded bg-dark-800 text-accent-cyan">stable-stream update</code></li>
                    </ul>
                </div>
            </div>
        </div>
    );
};

export default SettingsUpdate;
