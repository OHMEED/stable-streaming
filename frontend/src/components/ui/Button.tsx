import React, { type ButtonHTMLAttributes } from 'react';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
    size?: 'sm' | 'md' | 'lg';
    isLoading?: boolean;
}

export const Button: React.FC<ButtonProps> = ({
    children,
    variant = 'primary',
    size = 'md',
    isLoading = false,
    className = '',
    ...props
}) => {
    const baseStyles = "inline-flex items-center justify-center rounded-lg font-medium transition-all duration-300 focus:outline-none focus:ring-2 focus:ring-offset-2 dark:focus:ring-offset-dark-900 focus:ring-offset-light-100 disabled:opacity-50 disabled:cursor-not-allowed active:scale-95";

    const variants = {
        primary: "bg-gradient-to-r from-accent-red to-accent-red-light hover:from-accent-red-light hover:to-accent-red text-white shadow-lg shadow-accent-red/20 hover:shadow-accent-red/40 border border-transparent",
        secondary: "dark:bg-dark-700/50 bg-light-200/50 dark:hover:bg-dark-600 hover:bg-light-300 dark:text-dark-200 text-dark-700 border dark:border-dark-600 border-light-300 dark:hover:border-dark-500 hover:border-light-400 backdrop-blur-sm",
        danger: "bg-red-500/10 hover:bg-red-500/20 text-red-500 border border-red-500/50 hover:border-red-500",
        ghost: "bg-transparent dark:hover:bg-white/5 hover:bg-black/5 dark:text-dark-300 text-dark-600 hover:text-accent-red"
    };

    const sizes = {
        sm: "px-3 py-1.5 text-sm",
        md: "px-4 py-2 text-base",
        lg: "px-6 py-3 text-lg"
    };

    return (
        <button
            className={`${baseStyles} ${variants[variant]} ${sizes[size]} ${className}`}
            disabled={isLoading || props.disabled}
            {...props}
        >
            {isLoading && (
                <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
            )}
            {children}
        </button>
    );
};
