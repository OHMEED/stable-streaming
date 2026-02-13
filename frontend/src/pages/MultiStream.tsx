import React from 'react';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import api from '../lib/api';

interface Destination {
    id: number;
    name: string;
    rtmp_url: string;
    stream_key: string;
    enabled: boolean;
}

interface Config {
    enabled: boolean;
    max_destinations: number;
    available: boolean;
}

const MultiStream: React.FC = () => {
    const [config, setConfig] = React.useState<Config | null>(null);
    const [destinations, setDestinations] = React.useState<Destination[]>([]);
    const [isStreaming, setIsStreaming] = React.useState(false);
    const [loading, setLoading] = React.useState(true);
    const [actionLoading, setActionLoading] = React.useState(false);
    const [showAddForm, setShowAddForm] = React.useState(false);
    const [error, setError] = React.useState<string | null>(null);
    const [newDest, setNewDest] = React.useState({ name: '', rtmp_url: '', stream_key: '' });

    const fetchData = async () => {
        try {
            const [configRes, destsRes, statusRes] = await Promise.all([
                api.get('/multistream/config'),
                api.get('/multistream/destinations'),
                api.get('/obs/status')
            ]);
            setConfig(configRes.data);
            setDestinations(destsRes.data || []);
            setIsStreaming(statusRes.data?.streaming || false);
            setError(null);
        } catch (e) {
            console.error('Failed to fetch multistream data', e);
            setError('Failed to load multistream configuration');
        } finally {
            setLoading(false);
        }
    };

    React.useEffect(() => {
        fetchData();
        // Poll for streaming status
        const interval = setInterval(async () => {
            try {
                const statusRes = await api.get('/obs/status');
                setIsStreaming(statusRes.data?.streaming || false);
            } catch (e) {
                // Ignore polling errors
            }
        }, 5000);
        return () => clearInterval(interval);
    }, []);

    const handleEnable = async () => {
        setActionLoading(true);
        setError(null);
        try {
            await api.post('/multistream/enable');
            await fetchData();
        } catch (e: any) {
            setError(e.response?.data || 'Failed to enable multistream');
        } finally {
            setActionLoading(false);
        }
    };

    const handleDisable = async () => {
        setActionLoading(true);
        setError(null);
        try {
            await api.post('/multistream/disable');
            await fetchData();
        } catch (e: any) {
            setError(e.response?.data || 'Failed to disable multistream');
        } finally {
            setActionLoading(false);
        }
    };

    const handleAddDestination = async () => {
        if (!newDest.name || !newDest.rtmp_url || !newDest.stream_key) {
            setError('All fields are required');
            return;
        }
        setActionLoading(true);
        setError(null);
        try {
            await api.post('/multistream/destinations', newDest);
            setNewDest({ name: '', rtmp_url: '', stream_key: '' });
            setShowAddForm(false);
            await fetchData();
        } catch (e: any) {
            setError(e.response?.data || 'Failed to add destination');
        } finally {
            setActionLoading(false);
        }
    };

    const handleRemoveDestination = async (id: number) => {
        if (!confirm('Remove this destination?')) return;
        setActionLoading(true);
        setError(null);
        try {
            await api.delete(`/multistream/destinations/${id}`);
            await fetchData();
        } catch (e: any) {
            setError(e.response?.data || 'Failed to remove destination');
        } finally {
            setActionLoading(false);
        }
    };

    const handleToggleDestination = async (dest: Destination) => {
        setActionLoading(true);
        setError(null);
        try {
            await api.put(`/multistream/destinations/${dest.id}`, {
                name: dest.name,
                rtmp_url: dest.rtmp_url,
                stream_key: dest.stream_key,
                enabled: !dest.enabled
            });
            await fetchData();
        } catch (e: any) {
            setError(e.response?.data || 'Failed to toggle destination');
        } finally {
            setActionLoading(false);
        }
    };

    if (loading) {
        return (
            <div className="flex items-center justify-center h-64">
                <div className="text-dark-400 animate-pulse">Loading multistream settings...</div>
            </div>
        );
    }

    if (!config?.available) {
        return (
            <div className="max-w-4xl mx-auto space-y-6">
                <h1 className="text-2xl font-bold dark:text-white text-dark-900">Multi-Stream</h1>
                <Card className="p-6">
                    <div className="text-center py-8">
                        <div className="text-4xl mb-4">🔒</div>
                        <h2 className="text-xl font-semibold dark:text-white text-dark-900 mb-2">
                            Multi-Streaming Not Enabled
                        </h2>
                        <p className="dark:text-dark-400 text-dark-500 max-w-md mx-auto">
                            Multi-streaming was not enabled during server setup. To enable it, restart the server
                            and answer "y" to "Enable Multi-Streaming?" when prompted.
                        </p>
                    </div>
                </Card>
            </div>
        );
    }

    const canAddMore = destinations.length < (config?.max_destinations || 0);
    const isLocked = isStreaming;
    // Can only modify destinations when multistream is disabled (setup mode)
    // When enabled, the config is actively in use and should not be modified
    const canModifyDestinations = !isStreaming && !config?.enabled;

    return (
        <div className="max-w-4xl mx-auto space-y-6">
            <h1 className="text-2xl font-bold dark:text-white text-dark-900">Multi-Stream</h1>
            <p className="dark:text-dark-300 text-dark-600">
                Stream to multiple platforms simultaneously. When enabled, OBS streams to the internal relay,
                which forwards to all configured destinations.
            </p>

            {/* Error Display */}
            {error && (
                <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-4 text-red-400">
                    {error}
                </div>
            )}

            {/* Streaming Lock Warning */}
            {isLocked && (
                <div className="bg-amber-500/10 border border-amber-500/30 rounded-lg p-4 flex items-center gap-3">
                    <span className="text-2xl">🔴</span>
                    <div>
                        <div className="font-medium text-amber-400">Currently Streaming</div>
                        <div className="text-sm text-amber-400/70">
                            Stop the stream to modify multistream settings
                        </div>
                    </div>
                </div>
            )}

            {/* Enable/Disable Card */}
            <Card className="p-6">
                <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                    <div>
                        <h2 className="text-lg font-semibold dark:text-white text-dark-900">
                            {config?.enabled ? '🟢 Multi-Stream Enabled' : '⚪ Single Stream Mode'}
                        </h2>
                        <p className="text-sm dark:text-dark-400 text-dark-500 mt-1">
                            {config?.enabled
                                ? 'OBS is routing through the multi-stream relay. Start streaming to broadcast to all destinations.'
                                : 'Enable to switch OBS to the multi-stream relay and configure destinations.'}
                        </p>
                    </div>
                    <Button
                        variant={config?.enabled ? 'secondary' : 'primary'}
                        onClick={config?.enabled ? handleDisable : handleEnable}
                        disabled={isLocked || actionLoading}
                        className="whitespace-nowrap"
                    >
                        {actionLoading ? 'Processing...' : config?.enabled ? 'Disable' : 'Enable Multi-Stream'}
                    </Button>
                </div>
            </Card>

            {/* Destinations Card */}
            <Card className="p-6">
                <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6">
                    <div>
                        <h2 className="text-lg font-semibold dark:text-white text-dark-900">
                            Destinations
                        </h2>
                        <p className="text-sm dark:text-dark-400 text-dark-500">
                            {destinations.length} of {config?.max_destinations} destinations configured
                        </p>
                    </div>
                    {!config?.enabled && canAddMore && (
                        <Button
                            variant="primary"
                            size="sm"
                            onClick={() => setShowAddForm(true)}
                            disabled={isStreaming}
                        >
                            + Add Destination
                        </Button>
                    )}
                </div>

                {destinations.length === 0 ? (
                    <div className="text-center py-12">
                        <div className="text-4xl mb-3">📡</div>
                        <p className="dark:text-dark-400 text-dark-500">No destinations configured</p>
                        {!config?.enabled && (
                            <p className="text-sm dark:text-dark-500 text-dark-400 mt-2">
                                Add a destination to start multi-streaming
                            </p>
                        )}
                    </div>
                ) : (
                    <div className="space-y-3">
                        {destinations.map((dest) => (
                            <div
                                key={dest.id}
                                className={`flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-lg dark:bg-dark-700/50 bg-light-200/50 ${!dest.enabled ? 'opacity-50' : ''}`}
                            >
                                <div className="flex-1 min-w-0">
                                    <div className="flex items-center gap-2">
                                        <div className="font-medium dark:text-white text-dark-900 truncate">
                                            {dest.name}
                                        </div>
                                        {!dest.enabled && (
                                            <span className="text-xs px-2 py-0.5 rounded bg-dark-600 text-dark-300">Disabled</span>
                                        )}
                                    </div>
                                    <div className="text-xs dark:text-dark-400 text-dark-500 font-mono truncate">
                                        {dest.rtmp_url}
                                    </div>
                                    <div className="text-xs dark:text-dark-500 text-dark-400 font-mono">
                                        Key: ••••••••••{dest.stream_key.slice(-4)}
                                    </div>
                                </div>
                                <div className="flex items-center gap-3">
                                    {/* Toggle Switch */}
                                    <button
                                        onClick={() => handleToggleDestination(dest)}
                                        disabled={!canModifyDestinations || actionLoading}
                                        className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${dest.enabled
                                            ? 'bg-green-500'
                                            : 'bg-dark-600'
                                            } ${!canModifyDestinations ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer hover:opacity-80'}`}
                                        title={!canModifyDestinations ? 'Disable multistream to modify destinations' : (dest.enabled ? 'Disable' : 'Enable')}
                                    >
                                        <span
                                            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${dest.enabled ? 'translate-x-6' : 'translate-x-1'
                                                }`}
                                        />
                                    </button>
                                    <Button
                                        variant="ghost"
                                        size="sm"
                                        onClick={() => handleRemoveDestination(dest.id)}
                                        className="text-red-500 hover:text-red-400 hover:bg-red-500/10"
                                        disabled={actionLoading || !canModifyDestinations}
                                    >
                                        Remove
                                    </Button>
                                </div>
                            </div>
                        ))}
                    </div>
                )}

                {!canAddMore && !config?.enabled && (
                    <p className="text-center text-sm dark:text-dark-400 text-dark-500 mt-4">
                        Maximum destinations ({config?.max_destinations}) reached
                    </p>
                )}
            </Card>

            {/* Add Destination Modal */}
            {showAddForm && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
                    <Card className="p-6 w-full max-w-md space-y-4">
                        <h3 className="text-lg font-semibold dark:text-white text-dark-900">
                            Add Streaming Destination
                        </h3>
                        <p className="text-sm dark:text-dark-400 text-dark-500">
                            Enter the RTMP server details for your streaming platform.
                        </p>
                        <div className="space-y-3">
                            <div>
                                <label className="block text-sm font-medium dark:text-dark-300 text-dark-600 mb-1">
                                    Name
                                </label>
                                <input
                                    type="text"
                                    placeholder="e.g., Twitch, YouTube, Kick"
                                    value={newDest.name}
                                    onChange={(e) => setNewDest({ ...newDest, name: e.target.value })}
                                    className="w-full px-3 py-2 rounded-lg dark:bg-dark-700 bg-light-200 dark:text-white text-dark-900 border dark:border-dark-600 border-light-400"
                                />
                            </div>
                            <div>
                                <label className="block text-sm font-medium dark:text-dark-300 text-dark-600 mb-1">
                                    RTMP Server URL
                                </label>
                                <input
                                    type="text"
                                    placeholder="rtmp://live.twitch.tv/app"
                                    value={newDest.rtmp_url}
                                    onChange={(e) => setNewDest({ ...newDest, rtmp_url: e.target.value })}
                                    className="w-full px-3 py-2 rounded-lg dark:bg-dark-700 bg-light-200 dark:text-white text-dark-900 border dark:border-dark-600 border-light-400 font-mono text-sm"
                                />
                            </div>
                            <div>
                                <label className="block text-sm font-medium dark:text-dark-300 text-dark-600 mb-1">
                                    Stream Key
                                </label>
                                <input
                                    type="password"
                                    placeholder="Your stream key (kept secure)"
                                    value={newDest.stream_key}
                                    onChange={(e) => setNewDest({ ...newDest, stream_key: e.target.value })}
                                    className="w-full px-3 py-2 rounded-lg dark:bg-dark-700 bg-light-200 dark:text-white text-dark-900 border dark:border-dark-600 border-light-400"
                                />
                            </div>
                        </div>
                        <div className="flex gap-3 justify-end pt-2">
                            <Button
                                variant="secondary"
                                onClick={() => {
                                    setShowAddForm(false);
                                    setNewDest({ name: '', rtmp_url: '', stream_key: '' });
                                }}
                            >
                                Cancel
                            </Button>
                            <Button
                                variant="primary"
                                onClick={handleAddDestination}
                                disabled={actionLoading}
                            >
                                {actionLoading ? 'Adding...' : 'Add Destination'}
                            </Button>
                        </div>
                    </Card>
                </div>
            )}
        </div>
    );
};

export default MultiStream;
