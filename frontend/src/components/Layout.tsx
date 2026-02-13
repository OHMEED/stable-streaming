import React from 'react';
import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom';
import { Button } from './ui/Button';
import { useTheme } from '../contexts/ThemeContext';
import logoImage from '../assets/logo.png';
import api from '../lib/api';

const Layout: React.FC = () => {
    const navigate = useNavigate();
    const location = useLocation();
    const { theme, toggleTheme } = useTheme();
    const [isSidebarOpen, setIsSidebarOpen] = React.useState(false);
    const [multistreamAvailable, setMultistreamAvailable] = React.useState(false);

    // Check multistream availability on mount
    React.useEffect(() => {
        api.get('/multistream/config')
            .then(res => {
                setMultistreamAvailable(res.data?.available || false);
            })
            .catch(() => {
                // Silently fail - multistream just won't show
            });
    }, []);

    const handleLogout = () => {
        localStorage.removeItem('token');
        navigate('/login');
    };

    const toggleSidebar = () => setIsSidebarOpen(!isSidebarOpen);
    const closeSidebar = () => setIsSidebarOpen(false);

    const navLinkClass = ({ isActive }: { isActive: boolean }) => `
        flex items-center px-4 py-3 rounded-xl transition-all duration-300 font-medium text-sm
        ${isActive
            ? 'bg-accent-red/20 text-accent-red shadow-lg shadow-accent-red/10 border border-accent-red/30 dark:text-accent-red-light'
            : 'dark:text-dark-300 text-dark-600 hover:text-accent-red dark:hover:text-accent-red hover:bg-accent-red/5'}
    `;

    // Navigation Items Reuse
    const NavItems = () => (
        <nav className="flex flex-col space-y-1">
            <div className="px-4 text-xs font-semibold dark:text-dark-400 text-dark-500 uppercase tracking-wider mb-2">Menu</div>
            <NavLink to="/dashboard" className={navLinkClass} onClick={closeSidebar}>
                Dashboard
            </NavLink>
            <NavLink to="/obs" end className={navLinkClass} onClick={closeSidebar}>
                OBS Control
            </NavLink>
            <NavLink to="/obs/settings" className={navLinkClass} onClick={closeSidebar}>
                OBS Websocket
            </NavLink>
            {multistreamAvailable && (
                <NavLink to="/obs/multistream" className={navLinkClass} onClick={closeSidebar}>
                    Multi-Stream
                </NavLink>
            )}
            <NavLink to="/recordings" className={navLinkClass} onClick={closeSidebar}>
                Recordings
            </NavLink>

            <NavLink to="/settings/ingests" className={navLinkClass} onClick={closeSidebar}>
                Ingests
            </NavLink>
            <NavLink to="/settings/scene-switcher" className={navLinkClass} onClick={closeSidebar}>
                Auto Scene Switcher
            </NavLink>
            <NavLink to="/settings/logs" className={navLinkClass} onClick={closeSidebar}>
                Logs
            </NavLink>
            <NavLink to="/settings/password" className={navLinkClass} onClick={closeSidebar}>
                Password
            </NavLink>
        </nav>
    );

    const isOBSPage = location.pathname === '/obs';
    const isIngestPage = location.pathname === '/settings/ingests';

    return (
        <div className="min-h-screen flex flex-col font-sans bg-fixed bg-cover overflow-hidden">
            {/* Header */}
            <header className="glass h-16 sticky top-0 z-40 px-6 flex justify-between items-center shrink-0">
                <div className="flex items-center gap-3">
                    <button
                        onClick={toggleSidebar}
                        className="md:hidden p-2 dark:text-dark-300 text-dark-600 hover:text-accent-red transition-colors"
                        aria-label="Toggle Menu"
                    >
                        <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                        </svg>
                    </button>
                    {/* Logo */}
                    <img
                        src={logoImage}
                        alt="Stable Streaming"
                        className="h-6 xs:h-8 sm:h-10 w-auto object-contain max-w-[120px] sm:max-w-none"
                    />
                </div>
                <div className="flex gap-3 items-center">
                    {/* Theme Toggle */}
                    <button
                        onClick={toggleTheme}
                        className="p-2 rounded-lg dark:bg-dark-700/50 bg-light-200/50 dark:hover:bg-dark-600 hover:bg-light-300 transition-all duration-300 dark:text-accent-cyan text-dark-600"
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
                    <Button variant="ghost" size="sm" onClick={handleLogout} className="text-accent-red hover:text-accent-red-light hover:bg-accent-red/10">
                        Logout
                    </Button>
                </div>
            </header>

            <div className="flex flex-1 overflow-hidden h-[calc(100vh-64px)]">
                {/* Sidebar Navigation (Desktop) */}
                <aside className="w-64 glass border-r dark:border-dark-700/30 border-light-300/50 hidden md:flex flex-col pt-6 px-4 space-y-6 overflow-y-auto">
                    <NavItems />
                </aside>

                {/* Sidebar Navigation (Mobile Overlay) */}
                {isSidebarOpen && (
                    <div className="fixed inset-0 z-50 md:hidden flex">
                        <div
                            className="dark:bg-black/50 bg-black/30 backdrop-blur-sm flex-1"
                            onClick={closeSidebar}
                        ></div>
                        <aside className="w-64 dark:bg-dark-900 bg-light-100 border-l dark:border-dark-700/30 border-light-300/50 flex flex-col pt-6 px-4 space-y-6 overflow-y-auto h-full shadow-2xl animate-slide-in-right">
                            <div className="flex justify-between items-center mb-4 px-2">
                                <span className="font-bold dark:text-white text-dark-900">Menu</span>
                                <button onClick={closeSidebar} className="dark:text-dark-300 text-dark-600 hover:text-accent-red">
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                    </svg>
                                </button>
                            </div>
                            <NavItems />
                        </aside>
                    </div>
                )}

                {/* Main Content */}
                <main className={`flex-1 relative overflow-y-auto scrollbar-thin scrollbar-thumb-gray-700 scrollbar-track-transparent ${isOBSPage ? 'p-0' : 'p-6'}`}>
                    <div className={`space-y-6 animate-fade-in pb-10 ${isOBSPage ? 'h-full' : (isIngestPage ? 'w-full' : 'max-w-7xl mx-auto')}`}>
                        <Outlet />
                    </div>
                </main>
            </div>
        </div>
    );
};

export default Layout;
