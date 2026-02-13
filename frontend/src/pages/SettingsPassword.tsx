import React, { useState } from 'react';
import { Card } from '../components/ui/Card';
import api from '../lib/api';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';

const SettingsPassword: React.FC = () => {
    const [isLoading, setIsLoading] = useState(false);
    const [currentPassword, setCurrentPassword] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [confirmNewPassword, setConfirmNewPassword] = useState('');
    const [errMessage, setErrMessage] = useState('');

    const handlePasswordChange = async (e: React.FormEvent) => {
        e.preventDefault();
        setErrMessage('');

        if (newPassword !== confirmNewPassword) {
            alert("New passwords do not match");
            return;
        }

        setIsLoading(true);
        try {
            await api.post('/user/password', {
                current_password: currentPassword,
                new_password: newPassword
            });
            alert('Password updated successfully');
            setCurrentPassword('');
            setNewPassword('');
            setConfirmNewPassword('');
        } catch (err: any) {
            console.error(err);
            const msg = err.response?.data || err.message;
            setErrMessage(msg);
            alert('Failed to update password: ' + msg);
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div className="space-y-6">
            <h1 className="text-3xl font-bold dark:text-white text-dark-900 mb-8">Security</h1>

            <div className="max-w-xl">
                <Card>
                    <h2 className="text-xl font-bold dark:text-white text-dark-900 mb-6">Change Password</h2>
                    <form onSubmit={handlePasswordChange} className="space-y-4">
                        <Input
                            type="password"
                            label="Current Password"
                            placeholder="••••••••"
                            value={currentPassword}
                            onChange={(e: any) => setCurrentPassword(e.target.value)}
                            required
                        />
                        <Input
                            type="password"
                            label="New Password"
                            placeholder="••••••••"
                            value={newPassword}
                            onChange={(e: any) => setNewPassword(e.target.value)}
                            required
                        />
                        <Input
                            type="password"
                            label="Confirm New Password"
                            placeholder="••••••••"
                            value={confirmNewPassword}
                            onChange={(e: any) => setConfirmNewPassword(e.target.value)}
                            required
                        />

                        {errMessage && <div className="text-accent-red text-sm">{errMessage}</div>}

                        <div className="pt-4">
                            <Button type="submit" isLoading={isLoading}>
                                Update Password
                            </Button>
                        </div>
                    </form>
                </Card>
            </div>
        </div>
    );
};

export default SettingsPassword;
