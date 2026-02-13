import React, { type InputHTMLAttributes } from 'react';

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
    label?: string;
    error?: string;
}

export const Input: React.FC<InputProps> = ({
    label,
    error,
    className = '',
    id,
    ...props
}) => {
    return (
        <div className="w-full">
            {label && (
                <label htmlFor={id} className="block text-sm font-medium dark:text-dark-300 text-dark-600 mb-1.5 ml-1">
                    {label}
                </label>
            )}
            <input
                id={id}
                className={`
                    w-full px-4 py-2.5 dark:bg-dark-900/50 bg-white/50 border rounded-lg 
                    dark:text-dark-100 text-dark-900 dark:placeholder-dark-400 placeholder-dark-500
                    focus:outline-none focus:ring-2 focus:ring-accent-red/50 focus:border-accent-red transition-all duration-300
                    ${error
                        ? 'border-red-500/50 focus:border-red-500 focus:ring-red-500/20'
                        : 'dark:border-dark-600 border-light-300 focus:border-accent-red dark:hover:border-dark-500 hover:border-light-400'}
                    ${className}
                `}
                {...props}
            />
            {error && (
                <p className="mt-1 text-sm text-red-500 ml-1">{error}</p>
            )}
        </div>
    );
};
