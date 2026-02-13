import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import api from '../lib/api';
import { Card } from '../components/ui/Card';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { useTheme } from '../contexts/ThemeContext';
import logoImage from '../assets/logo.png';

const Login: React.FC = () => {
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const navigate = useNavigate();
    const { theme, toggleTheme } = useTheme();

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        setIsLoading(true);
        setError('');

        try {
            const res = await api.post('/login', { username, password });
            localStorage.setItem('token', res.data.token);
            navigate('/dashboard');
        } catch (err) {
            setError('Invalid credentials');
            setIsLoading(false);
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center p-4 relative overflow-hidden">
            {/* Theme Toggle in corner */}
            <button
                onClick={toggleTheme}
                className="absolute top-4 right-4 p-2 rounded-lg dark:bg-dark-700/50 bg-light-200/50 dark:hover:bg-dark-600 hover:bg-light-300 transition-all duration-300 dark:text-accent-cyan text-dark-600 z-20"
                aria-label="Toggle Theme"
            >
                {theme === 'dark' ? (
                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
                    </svg>
                ) : (
                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
                    </svg>
                )}
            </button>

            {/* Background enhancement */}
            <div className="absolute inset-0 dark:bg-gradient-to-br dark:from-dark-900 dark:via-dark-800 dark:to-dark-900 bg-gradient-to-br from-light-100 via-light-200 to-light-100">
                {/* Accent glow */}
                <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-accent-red/10 rounded-full blur-3xl"></div>
                <div className="absolute bottom-1/4 right-1/4 w-64 h-64 bg-accent-cyan/10 rounded-full blur-3xl"></div>
            </div>

            <Card className="w-full max-w-md relative z-10 animate-slide-up border-accent-red/20 shadow-2xl dark:shadow-accent-red/10 shadow-accent-red/5 backdrop-blur-xl dark:bg-dark-900/90 bg-white/90">
                <div className="text-center mb-8">
                    <div className="flex justify-center mb-6">
                        <img
                            src={logoImage}
                            alt="Stable Streaming"
                            className="h-16 w-auto object-contain"
                        />
                    </div>
                    <h1 className="text-3xl font-bold dark:text-white text-dark-900 mb-2 tracking-tight">Welcome Back</h1>
                    <p className="dark:text-dark-300 text-dark-600">Sign in to manage your stable streams</p>
                </div>

                <form onSubmit={handleLogin} className="space-y-6">
                    <Input
                        label="Username"
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        placeholder="admin"
                        autoFocus
                    />

                    <Input
                        type="password"
                        label="Password"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        placeholder="••••••••"
                        error={error}
                    />

                    <Button type="submit" className="w-full font-bold text-lg" size="lg" isLoading={isLoading}>
                        Login to Dashboard
                    </Button>
                </form>
            </Card>
        </div>
    );
};

export default Login;
