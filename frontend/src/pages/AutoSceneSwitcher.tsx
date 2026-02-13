import React, { useEffect, useState } from 'react';
import api from '../lib/api';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';

interface Ingest {
    id: number;
    name: string;
    protocol: string;
    is_running: boolean;
}

interface SceneSwitcherConfig {
    ingest_id: number;
    online_scene: string;
    offline_scene: string;
    only_on_scene: string;
    threshold_kbps: number;
    enabled: boolean;
    running?: boolean;
}

const AutoSceneSwitcher: React.FC = () => {
    const [ingests, setIngests] = useState<Ingest[]>([]);
    const [scenes, setScenes] = useState<string[]>([]);
    const [config, setConfig] = useState<SceneSwitcherConfig>({
        ingest_id: 0,
        online_scene: '',
        offline_scene: '',
        only_on_scene: '',
        threshold_kbps: 1000,
        enabled: false,
    });
    const [isRunning, setIsRunning] = useState(false);
    const [isLoading, setIsLoading] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [thresholdUnit, setThresholdUnit] = useState<'kbps' | 'mbps'>('kbps');
    const [displayThreshold, setDisplayThreshold] = useState('1000');

    useEffect(() => {
        fetchData();
    }, []);

    const fetchData = async () => {
        setIsLoading(true);
        try {
            const [ingestsRes, scenesRes, configRes] = await Promise.all([
                api.get('/ingests'),
                api.get('/obs/scenes'),
                api.get('/scene-switcher/config'),
            ]);

            setIngests(ingestsRes.data || []);
            setScenes(scenesRes.data || []);

            if (configRes.data) {
                setConfig(configRes.data);
                setIsRunning(configRes.data.running || false);
                // Set display threshold based on value
                const kbps = configRes.data.threshold_kbps || 1000;
                if (kbps >= 1000 && kbps % 1000 === 0) {
                    setThresholdUnit('mbps');
                    setDisplayThreshold(String(kbps / 1000));
                } else {
                    setThresholdUnit('kbps');
                    setDisplayThreshold(String(kbps));
                }
            }
        } catch (err) {
            console.error('Failed to fetch data:', err);
        } finally {
            setIsLoading(false);
        }
    };

    const handleThresholdChange = (value: string) => {
        setDisplayThreshold(value);
        const numValue = parseFloat(value) || 0;
        const kbps = thresholdUnit === 'mbps' ? numValue * 1000 : numValue;
        setConfig(prev => ({ ...prev, threshold_kbps: Math.round(kbps) }));
    };

    const handleUnitChange = (unit: 'kbps' | 'mbps') => {
        const currentKbps = config.threshold_kbps;
        setThresholdUnit(unit);
        if (unit === 'mbps') {
            setDisplayThreshold(String(currentKbps / 1000));
        } else {
            setDisplayThreshold(String(currentKbps));
        }
    };

    const isFormValid = () => {
        return (
            config.ingest_id > 0 &&
            config.online_scene !== '' &&
            config.offline_scene !== '' &&
            config.threshold_kbps > 0
        );
    };

    const handleSave = async () => {
        if (!isFormValid()) {
            alert('Please fill in all required fields.');
            return;
        }

        setIsSaving(true);
        try {
            await api.post('/scene-switcher/config', config);
            await fetchData(); // Refresh to get running status
            alert('Configuration saved successfully!');
        } catch (err: any) {
            console.error('Failed to save config:', err);
            alert(err.response?.data || 'Failed to save configuration.');
        } finally {
            setIsSaving(false);
        }
    };

    const handleToggleEnabled = async () => {
        if (!config.enabled && !isFormValid()) {
            alert('Please save a valid configuration before enabling.');
            return;
        }

        try {
            const newEnabled = !config.enabled;
            await api.post('/scene-switcher/enable', { enabled: newEnabled });
            setConfig(prev => ({ ...prev, enabled: newEnabled }));
            setIsRunning(newEnabled);
        } catch (err: any) {
            console.error('Failed to toggle enabled:', err);
            alert(err.response?.data || 'Failed to toggle scene switcher.');
        }
    };

    const selectedIngest = ingests.find(i => i.id === config.ingest_id);

    return (
        <div className="space-y-6 max-w-3xl mx-auto">
            <div className="flex justify-between items-center">
                <h2 className="text-2xl font-bold dark:text-white text-dark-900">
                    Auto Scene Switcher
                </h2>
                <Button variant="secondary" onClick={fetchData} size="sm" disabled={isLoading}>
                    {isLoading ? 'Loading...' : 'Refresh'}
                </Button>
            </div>

            <Card className="p-6">
                {/* Description */}
                <div className="mb-6 p-4 rounded-lg dark:bg-dark-800/50 bg-light-200/50 border dark:border-dark-700/50 border-light-300">
                    <p className="dark:text-dark-300 text-dark-600 text-sm leading-relaxed">
                        Automatically switch OBS scenes based on ingest bitrate. When the selected ingest
                        falls below the threshold, OBS will switch to the <strong>Offline Scene</strong>.
                        When receiving above the threshold, it switches to the <strong>Online Scene</strong>.
                    </p>
                </div>

                {/* Status Banner */}
                <div className={`mb-6 p-4 rounded-lg border flex items-center justify-between ${config.enabled
                    ? 'dark:bg-green-900/20 bg-green-50 dark:border-green-500/30 border-green-200'
                    : 'dark:bg-dark-800/30 bg-light-100 dark:border-dark-700/50 border-light-300'
                    }`}>
                    <div className="flex items-center gap-3">
                        <div className={`w-3 h-3 rounded-full ${isRunning
                            ? 'bg-green-500 animate-pulse'
                            : 'dark:bg-dark-500 bg-light-400'
                            }`} />
                        <span className={`font-medium ${config.enabled
                            ? 'dark:text-green-400 text-green-600'
                            : 'dark:text-dark-400 text-dark-600'
                            }`}>
                            {isRunning ? 'Active - Monitoring' : config.enabled ? 'Enabled' : 'Disabled'}
                        </span>
                    </div>
                    <Button
                        variant={config.enabled ? 'danger' : 'primary'}
                        size="sm"
                        onClick={handleToggleEnabled}
                    >
                        {config.enabled ? 'Disable' : 'Enable'}
                    </Button>
                </div>

                {/* Configuration Form */}
                <div className={`space-y-6 ${config.enabled ? 'opacity-50 pointer-events-none' : ''}`}>
                    {/* Ingest Selection */}
                    <div>
                        <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-2">
                            Monitor Ingest <span className="text-red-400">*</span>
                        </label>
                        <select
                            className="w-full dark:bg-dark-900/50 bg-light-200/50 border dark:border-dark-600 border-light-300 rounded-lg px-4 py-3 dark:text-white text-dark-900 focus:outline-none focus:ring-2 focus:ring-accent-red"
                            value={config.ingest_id}
                            onChange={e => setConfig(prev => ({ ...prev, ingest_id: parseInt(e.target.value) || 0 }))}
                            disabled={config.enabled}
                        >
                            <option value={0}>Select an ingest to monitor...</option>
                            {ingests.map(ingest => (
                                <option key={ingest.id} value={ingest.id}>
                                    {ingest.name} ({ingest.protocol.toUpperCase()})
                                    {!ingest.is_running && ' - Not Running'}
                                </option>
                            ))}
                        </select>
                        {selectedIngest && !selectedIngest.is_running && (
                            <p className="mt-2 text-sm text-yellow-500">
                                ⚠️ This ingest is not currently running. Start it for scene switching to work.
                            </p>
                        )}
                    </div>

                    {/* Scene Selections */}
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <div>
                            <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-2">
                                Online Scene <span className="text-red-400">*</span>
                            </label>
                            <select
                                className="w-full dark:bg-dark-900/50 bg-light-200/50 border dark:border-dark-600 border-light-300 rounded-lg px-4 py-3 dark:text-white text-dark-900 focus:outline-none focus:ring-2 focus:ring-accent-red"
                                value={config.online_scene}
                                onChange={e => setConfig(prev => ({ ...prev, online_scene: e.target.value }))}
                                disabled={config.enabled}
                            >
                                <option value="">Select scene for good connection...</option>
                                {scenes.map(scene => (
                                    <option key={scene} value={scene}>{scene}</option>
                                ))}
                            </select>
                            <p className="mt-1 text-xs dark:text-dark-400 text-dark-500">
                                Shown when bitrate is above threshold
                            </p>
                        </div>

                        <div>
                            <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-2">
                                Offline Scene <span className="text-red-400">*</span>
                            </label>
                            <select
                                className="w-full dark:bg-dark-900/50 bg-light-200/50 border dark:border-dark-600 border-light-300 rounded-lg px-4 py-3 dark:text-white text-dark-900 focus:outline-none focus:ring-2 focus:ring-accent-red"
                                value={config.offline_scene}
                                onChange={e => setConfig(prev => ({ ...prev, offline_scene: e.target.value }))}
                                disabled={config.enabled}
                            >
                                <option value="">Select scene for low/no connection...</option>
                                {scenes.map(scene => (
                                    <option key={scene} value={scene}>{scene}</option>
                                ))}
                            </select>
                            <p className="mt-1 text-xs dark:text-dark-400 text-dark-500">
                                Shown when bitrate is below threshold
                            </p>
                        </div>
                    </div>

                    {/* Threshold */}
                    <div>
                        <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-2">
                            Bitrate Threshold <span className="text-red-400">*</span>
                        </label>
                        <div className="flex gap-3">
                            <div className="flex-1">
                                <Input
                                    type="number"
                                    min="1"
                                    step={thresholdUnit === 'mbps' ? '0.1' : '100'}
                                    value={displayThreshold}
                                    onChange={e => handleThresholdChange(e.target.value)}
                                    placeholder={thresholdUnit === 'mbps' ? '1.0' : '1000'}
                                    disabled={config.enabled}
                                    className="text-lg"
                                />
                            </div>
                            <select
                                className="dark:bg-dark-900/50 bg-light-200/50 border dark:border-dark-600 border-light-300 rounded-lg px-4 py-3 dark:text-white text-dark-900 focus:outline-none focus:ring-2 focus:ring-accent-red"
                                value={thresholdUnit}
                                onChange={e => handleUnitChange(e.target.value as 'kbps' | 'mbps')}
                                disabled={config.enabled}
                            >
                                <option value="kbps">kbps</option>
                                <option value="mbps">Mbps</option>
                            </select>
                        </div>
                        <p className="mt-2 text-xs dark:text-dark-400 text-dark-500">
                            Switch to Offline Scene when bitrate falls below {config.threshold_kbps} kbps
                            ({(config.threshold_kbps / 1000).toFixed(2)} Mbps)
                        </p>
                    </div>

                    {/* Only when On Scene (Optional) */}
                    <div>
                        <label className="block text-sm font-medium dark:text-dark-300 text-dark-700 mb-2">
                            Only when On Scene <span className="text-dark-500 font-normal">(Optional)</span>
                        </label>
                        <select
                            className="w-full dark:bg-dark-900/50 bg-light-200/50 border dark:border-dark-600 border-light-300 rounded-lg px-4 py-3 dark:text-white text-dark-900 focus:outline-none focus:ring-2 focus:ring-accent-red"
                            value={config.only_on_scene}
                            onChange={e => setConfig(prev => ({ ...prev, only_on_scene: e.target.value }))}
                            disabled={config.enabled}
                        >
                            <option value="">Always active (no restriction)</option>
                            {scenes.map(scene => (
                                <option key={scene} value={scene}>{scene}</option>
                            ))}
                        </select>
                        <p className="mt-1 text-xs dark:text-dark-400 text-dark-500">
                            When set, auto-switching only occurs when OBS is on this scene (or Online/Offline scenes).
                            If you manually switch to another scene, the switcher won't interfere.
                        </p>
                    </div>

                    {/* Save Button */}
                    <div className="pt-4 border-t dark:border-dark-700 border-light-300">
                        <Button
                            onClick={handleSave}
                            disabled={!isFormValid() || isSaving || config.enabled}
                            className="w-full"
                            isLoading={isSaving}
                        >
                            {isSaving ? 'Saving...' : 'Save Configuration'}
                        </Button>
                        {config.enabled && (
                            <p className="mt-3 text-center text-sm dark:text-dark-400 text-dark-500">
                                Disable to modify settings
                            </p>
                        )}
                    </div>
                </div>
            </Card>

            {/* Help Card */}
            <Card className="p-6">
                <h3 className="text-lg font-semibold dark:text-white text-dark-900 mb-3">How It Works</h3>
                <ul className="space-y-2 dark:text-dark-300 text-dark-600 text-sm">
                    <li className="flex gap-2">
                        <span className="text-accent-red">1.</span>
                        Select the ingest you want to monitor for bitrate changes.
                    </li>
                    <li className="flex gap-2">
                        <span className="text-accent-red">2.</span>
                        Choose which OBS scene to show when connection is good (Online) or poor (Offline).
                    </li>
                    <li className="flex gap-2">
                        <span className="text-accent-red">3.</span>
                        Set a bitrate threshold. When bitrate drops below this value, it switches to Offline.
                    </li>
                    <li className="flex gap-2">
                        <span className="text-accent-red">4.</span>
                        Save your configuration, then click <strong>Enable</strong> to start automatic switching.
                    </li>
                </ul>
            </Card>
        </div>
    );
};

export default AutoSceneSwitcher;
