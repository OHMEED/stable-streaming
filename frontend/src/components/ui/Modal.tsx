import React from 'react';

interface ModalProps {
    isOpen: boolean;
    onClose: () => void;
    title: string;
    children: React.ReactNode;
    footer?: React.ReactNode;
}

export const Modal: React.FC<ModalProps> = ({
    isOpen,
    onClose,
    title,
    children,
    footer
}) => {
    if (!isOpen) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center overflow-x-hidden overflow-y-auto outline-none focus:outline-none">
            {/* Backdrop */}
            <div
                className="fixed inset-0 dark:bg-black/60 bg-black/40 backdrop-blur-sm transition-opacity"
                onClick={onClose}
            ></div>

            {/* Modal Content */}
            <div className="relative w-full max-w-lg mx-auto my-6 z-50 animate-slide-up">
                <div className="relative flex flex-col w-full dark:bg-dark-900 bg-white border dark:border-dark-700/50 border-light-300 rounded-xl shadow-2xl outline-none focus:outline-none overflow-hidden">
                    {/* Header */}
                    <div className="flex items-start justify-between p-5 border-b dark:border-dark-700/50 border-light-300 dark:bg-dark-800/30 bg-light-100/50">
                        <h3 className="text-xl font-semibold dark:text-white text-dark-900">
                            {title}
                        </h3>
                        <button
                            className="p-1 ml-auto bg-transparent border-0 dark:text-dark-300 text-dark-600 hover:text-accent-red float-right leading-none outline-none focus:outline-none"
                            onClick={onClose}
                        >
                            <span className="text-2xl block">×</span>
                        </button>
                    </div>

                    {/* Body */}
                    <div className="relative p-6 flex-auto">
                        {children}
                    </div>

                    {/* Footer */}
                    {footer && (
                        <div className="flex items-center justify-end p-4 border-t dark:border-dark-700/50 border-light-300 dark:bg-dark-800/30 bg-light-100/50 gap-2">
                            {footer}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
};
