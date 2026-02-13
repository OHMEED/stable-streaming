import React from 'react';

interface BadgeProps {
    children: React.ReactNode;
    variant?: 'default' | 'success' | 'warning' | 'error' | 'info';
    size?: 'sm' | 'md';
    pulse?: boolean;
    className?: string;
}

export const Badge: React.FC<BadgeProps> = ({
    children,
    variant = 'default',
    size = 'md',
    pulse = false,
    className = ''
}) => {
    const variants = {
        default: "dark:bg-dark-700/50 bg-light-200/50 dark:text-dark-300 text-dark-600 dark:border-dark-600 border-light-400",
        success: "bg-green-500/10 text-green-400 border-green-500/20",
        warning: "bg-yellow-500/10 text-yellow-400 border-yellow-500/20",
        error: "bg-red-500/10 text-red-400 border-red-500/20",
        info: "bg-accent-cyan/10 text-accent-cyan border-accent-cyan/20"
    };

    const sizes = {
        sm: "px-2 py-0.5 text-xs",
        md: "px-2.5 py-0.5 text-sm"
    };

    return (
        <span className={`
            inline-flex items-center justify-center font-medium rounded-full border backdrop-blur-sm
            ${variants[variant]} 
            ${sizes[size]} 
            ${className}
        `}>
            {pulse && (
                <span className="flex h-2 w-2 mr-1.5 relative">
                    <span className={`animate-ping absolute inline-flex h-full w-full rounded-full opacity-75 ${variant === 'success' ? 'bg-green-400' : variant === 'error' ? 'bg-red-400' : 'bg-accent-cyan'}`}></span>
                    <span className={`relative inline-flex rounded-full h-2 w-2 ${variant === 'success' ? 'bg-green-500' : variant === 'error' ? 'bg-red-500' : 'bg-accent-cyan'}`}></span>
                </span>
            )}
            {children}
        </span>
    );
};
