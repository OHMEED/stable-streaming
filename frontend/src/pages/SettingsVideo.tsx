import React from 'react';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';

const SettingsVideo: React.FC = () => {
    return (
        <div className="space-y-6">
            <h1 className="text-3xl font-bold dark:text-white text-dark-900 mb-8">Video Settings</h1>

            <Card>
                <div className="text-center py-12">
                    <div className="w-16 h-16 dark:bg-dark-800/50 bg-light-200/50 rounded-full flex items-center justify-center mx-auto mb-4 border dark:border-dark-700 border-light-300">
                        <svg className="w-8 h-8 text-accent-red" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                    </div>
                    <h2 className="text-xl font-bold dark:text-white text-dark-900 mb-2">Video Configuration</h2>
                    <p className="dark:text-dark-300 text-dark-600 mb-6 max-w-md mx-auto">
                        Global video settings for the output stream. These settings affect the final broadcast quality.
                    </p>
                    <Button disabled variant="secondary">
                        Configure in OBS (Coming Soon)
                    </Button>
                </div>
            </Card>
        </div>
    );
};

export default SettingsVideo;
