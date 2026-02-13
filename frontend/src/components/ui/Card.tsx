import React, { type HTMLAttributes } from 'react';

interface CardProps extends HTMLAttributes<HTMLDivElement> {
    hoverEffect?: boolean;
}

export const Card: React.FC<CardProps> = ({
    children,
    className = '',
    hoverEffect = false,
    ...props
}) => {
    return (
        <div
            className={`
                dark:bg-dark-900/60 bg-white/60 backdrop-blur-md 
                border dark:border-dark-700/50 border-light-300/50 
                rounded-xl p-6
                ${hoverEffect ? 'dark:hover:bg-dark-800/70 hover:bg-white/80 transition-all duration-300 dark:hover:border-dark-600/80 hover:border-light-400/80 hover:shadow-lg dark:hover:shadow-accent-red/5 hover:shadow-accent-red/10' : ''}
                ${className}
            `}
            {...props}
        >
            {children}
        </div>
    );
};
