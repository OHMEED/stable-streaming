import React from 'react';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { QRCodeSVG } from 'qrcode.react';
import api from '../lib/api';

interface OBSSettings {
    password: string;
    port: number;
    server_ip: string;
}

const OBSSettingsPage: React.FC = () => {
    const [settings, setSettings] = React.useState<OBSSettings | null>(null);
    const [loading, setLoading] = React.useState(true);
    const [generating, setGenerating] = React.useState(false);
    const [showConfirmDialog, setShowConfirmDialog] = React.useState(false);
    const [copySuccess, setCopySuccess] = React.useState<string | null>(null);

    const fetchSettings = async () => {
        try {
            const res = await api.get('/obs/settings');
            setSettings(res.data);
        } catch (e) {
            console.error('Failed to fetch OBS settings', e);
        } finally {
            setLoading(false);
        }
    };

    React.useEffect(() => {
        fetchSettings();
    }, []);

    const generatePassword = async () => {
        setGenerating(true);
        try {
            const res = await fetch('/api/obs/settings/password', { method: 'POST' });
            if (res.ok) {
                const data = await res.json();
                setSettings(data);
            }
        } catch (e) {
            console.error('Failed to generate password', e);
        } finally {
            setGenerating(false);
            setShowConfirmDialog(false);
        }
    };

    const copyToClipboard = (text: string, label: string) => {
        navigator.clipboard.writeText(text);
        setCopySuccess(label);
        setTimeout(() => setCopySuccess(null), 2000);
    };

    // Build OBS WebSocket connection URL for QR code
    // Format: obsws://IP:PORT/PASSWORD
    const getConnectionUrl = () => {
        if (!settings) return '';
        return `obsws://${settings.server_ip}:${settings.port}/${settings.password}`;
    };

    if (loading) {
        return (
            <div className="flex items-center justify-center h-64">
                <div className="text-dark-400 animate-pulse">Loading OBS settings...</div>
            </div>
        );
    }

    return (
        <div className="space-y-6 max-w-4xl mx-auto">
            <h1 className="text-2xl font-bold dark:text-white text-dark-900">OBS Websocket</h1>
            <p className="dark:text-dark-300 text-dark-600">
                Configure OBS WebSocket connection for remote control via apps like OBS Blade or Moblin.
            </p>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                {/* Password Management Card */}
                <Card className="p-6 space-y-4">
                    <h2 className="text-lg font-semibold dark:text-white text-dark-900">WebSocket Password</h2>
                    <p className="text-sm dark:text-dark-400 text-dark-500">
                        This password is used to authenticate OBS WebSocket connections.
                    </p>

                    {/* Password Field */}
                    <div className="space-y-2">
                        <label className="text-sm font-medium dark:text-dark-300 text-dark-600">
                            Current Password
                        </label>
                        <div className="flex gap-2">
                            <input
                                type="text"
                                readOnly
                                value={settings?.password || ''}
                                className="flex-1 px-4 py-2 rounded-lg dark:bg-dark-700 bg-light-200 dark:text-white text-dark-900 border dark:border-dark-600 border-light-400 font-mono text-sm"
                            />
                            <Button
                                variant="secondary"
                                size="sm"
                                onClick={() => copyToClipboard(settings?.password || '', 'Password')}
                                className="whitespace-nowrap"
                            >
                                {copySuccess === 'Password' ? '✓ Copied' : 'Copy'}
                            </Button>
                        </div>
                    </div>

                    {/* Generate Button */}
                    <div className="pt-2">
                        <Button
                            variant="primary"
                            onClick={() => setShowConfirmDialog(true)}
                            disabled={generating}
                            className="w-full"
                        >
                            {generating ? 'Generating...' : 'Generate New Password'}
                        </Button>
                        <p className="text-xs dark:text-dark-500 text-dark-400 mt-2 text-center">
                            OBS may need to restart for changes to take effect
                        </p>
                    </div>
                </Card>

                {/* Connection Info Card */}
                <Card className="p-6 space-y-4">
                    <h2 className="text-lg font-semibold dark:text-white text-dark-900">Connection Info</h2>
                    <p className="text-sm dark:text-dark-400 text-dark-500">
                        Use these details to connect remote apps like OBS Blade or Moblin.
                    </p>

                    <div className="space-y-3">
                        {/* Server IP */}
                        <div className="flex items-center justify-between p-3 rounded-lg dark:bg-dark-700/50 bg-light-200/50">
                            <div>
                                <div className="text-xs dark:text-dark-400 text-dark-500">Server IP</div>
                                <div className="font-mono dark:text-white text-dark-900">{settings?.server_ip}</div>
                            </div>
                            <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => copyToClipboard(settings?.server_ip || '', 'IP')}
                            >
                                {copySuccess === 'IP' ? '✓' : 'Copy'}
                            </Button>
                        </div>

                        {/* Port */}
                        <div className="flex items-center justify-between p-3 rounded-lg dark:bg-dark-700/50 bg-light-200/50">
                            <div>
                                <div className="text-xs dark:text-dark-400 text-dark-500">Port</div>
                                <div className="font-mono dark:text-white text-dark-900">{settings?.port}</div>
                            </div>
                            <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => copyToClipboard(String(settings?.port), 'Port')}
                            >
                                {copySuccess === 'Port' ? '✓' : 'Copy'}
                            </Button>
                        </div>
                    </div>
                </Card>
            </div>

            {/* QR Code Card */}
            <Card className="p-6">
                <div className="flex flex-col lg:flex-row gap-6 items-center">
                    {/* QR Code */}
                    <div className="p-4 bg-white rounded-xl shadow-inner">
                        <QRCodeSVG
                            value={getConnectionUrl()}
                            size={180}
                            level="M"
                            includeMargin={false}
                        />
                    </div>

                    {/* Instructions */}
                    <div className="flex-1 space-y-4">
                        <h2 className="text-lg font-semibold dark:text-white text-dark-900">Connect with QR Code</h2>
                        <p className="dark:text-dark-300 text-dark-600">
                            Scan this QR code with a compatible app to instantly connect to OBS WebSocket.
                        </p>
                        <div className="space-y-2">
                            <h3 className="text-sm font-medium dark:text-dark-200 text-dark-700">Recommended Apps:</h3>
                            <ul className="text-sm dark:text-dark-400 text-dark-500 space-y-1">
                                <li className="flex items-center gap-2">
                                    <span className="w-2 h-2 bg-accent-cyan rounded-full"></span>
                                    <strong>OBS Blade</strong> - iOS & Android
                                </li>
                                <li className="flex items-center gap-2">
                                    <span className="w-2 h-2 bg-accent-cyan rounded-full"></span>
                                    <strong>Moblin</strong> - iOS streaming app with OBS remote
                                </li>
                            </ul>
                        </div>
                        <div className="p-3 rounded-lg dark:bg-dark-700/50 bg-light-200/50">
                            <div className="text-xs dark:text-dark-400 text-dark-500 mb-1">Connection URL</div>
                            <code className="text-xs dark:text-accent-cyan text-accent-cyan-dark break-all">
                                {getConnectionUrl()}
                            </code>
                        </div>
                    </div>
                </div>
            </Card>

            {/* Confirm Dialog */}
            {showConfirmDialog && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
                    <Card className="p-6 max-w-md mx-4 space-y-4">
                        <h3 className="text-lg font-semibold dark:text-white text-dark-900">
                            Generate New Password?
                        </h3>
                        <p className="dark:text-dark-300 text-dark-600">
                            This will replace the current password. Any connected apps will need to reconnect with the new password.
                        </p>
                        <div className="flex gap-3 justify-end">
                            <Button
                                variant="secondary"
                                onClick={() => setShowConfirmDialog(false)}
                            >
                                Cancel
                            </Button>
                            <Button
                                variant="primary"
                                onClick={generatePassword}
                                disabled={generating}
                            >
                                {generating ? 'Generating...' : 'Generate'}
                            </Button>
                        </div>
                    </Card>
                </div>
            )}
        </div>
    );
};

export default OBSSettingsPage;
