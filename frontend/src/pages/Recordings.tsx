import React, { useEffect, useState } from 'react';
import api from '../lib/api';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';

interface Recording {
    name: string;
    size: number;
    date: string;
}

interface StorageData {
    total: number;
    used: number;
    free: number;
}

const Recordings: React.FC = () => {
    const [recordings, setRecordings] = useState<Recording[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [selectedVideo, setSelectedVideo] = useState<string | null>(null);
    const [recordingToDelete, setRecordingToDelete] = useState<string | null>(null);
    const [isDeleting, setIsDeleting] = useState(false);
    const [sortBy, setSortBy] = useState<'date' | 'size' | 'name'>('date');
    const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
    const [storage, setStorage] = useState<StorageData | null>(null);

    useEffect(() => {
        fetchRecordings();
        fetchStorage();
    }, []);

    const fetchStorage = async () => {
        try {
            const res = await api.get('/system/storage');
            setStorage(res.data);
        } catch (err) {
            console.error("Failed to fetch storage data", err);
        }
    };

    const fetchRecordings = async () => {
        setIsLoading(true);
        try {
            const res = await api.get('/recordings');
            setRecordings(res.data || []);
        } catch (err) {
            console.error("Failed to fetch recordings", err);
        } finally {
            setIsLoading(false);
        }
    };

    const formatSize = (bytes: number) => {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

    const formatDate = (dateStr: string) => {
        return new Date(dateStr).toLocaleString();
    };

    const sortedRecordings = [...recordings].sort((a, b) => {
        let valA: any = a[sortBy];
        let valB: any = b[sortBy];

        if (sortBy === 'date') {
            valA = new Date(valA).getTime();
            valB = new Date(valB).getTime();
        }

        if (valA < valB) return sortOrder === 'asc' ? -1 : 1;
        if (valA > valB) return sortOrder === 'asc' ? 1 : -1;
        return 0;
    });

    const toggleSort = (field: 'date' | 'size' | 'name') => {
        if (sortBy === field) {
            setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
        } else {
            setSortBy(field);
            setSortOrder('desc');
        }
    };

    const handleDownload = (filename: string) => {
        const url = `${api.defaults.baseURL}/recordings/download/${encodeURIComponent(filename)}`;
        window.open(url, '_blank');
    };

    const handleDelete = async () => {
        if (!recordingToDelete) return;
        setIsDeleting(true);
        try {
            await api.delete(`/recordings/${encodeURIComponent(recordingToDelete)}`);
            setRecordings(prev => prev.filter(r => r.name !== recordingToDelete));
            setRecordingToDelete(null);
            fetchStorage();
        } catch (err) {
            console.error("Failed to delete recording", err);
            alert("Failed to delete recording");
        } finally {
            setIsDeleting(false);
        }
    };

    return (
        <div className="space-y-6">
            {/* Header - Stacks on mobile */}
            <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
                <div className="flex flex-col sm:flex-row items-start sm:items-center gap-4 sm:gap-6 w-full sm:w-auto">
                    <div className="flex justify-between items-center w-full sm:w-auto">
                        <h1 className="text-xl sm:text-2xl font-bold dark:text-white text-dark-900">Recordings</h1>
                        {/* Refresh button shown on mobile next to title */}
                        <Button
                            onClick={() => {
                                fetchRecordings();
                                fetchStorage();
                            }}
                            variant="secondary"
                            size="sm"
                            className="sm:hidden"
                        >
                            Refresh
                        </Button>
                    </div>
                    {storage && (
                        <div className="flex items-center gap-3 dark:bg-dark-900/50 bg-light-200/50 backdrop-blur-sm px-3 sm:px-4 py-2 rounded-xl border dark:border-dark-700 border-light-300 w-full sm:w-auto">
                            <div className="flex flex-col gap-1 w-full sm:w-auto">
                                <div className="flex justify-between text-[10px] uppercase tracking-wider font-bold dark:text-dark-400 text-dark-500">
                                    <span>Storage</span>
                                    <span>{Math.round((storage.used / storage.total) * 100)}% Used</span>
                                </div>
                                <div className="w-full sm:w-48 h-2 dark:bg-dark-700 bg-light-300 rounded-full overflow-hidden">
                                    <div
                                        className="h-full bg-accent-red transition-all duration-1000 ease-out"
                                        style={{ width: `${(storage.used / storage.total) * 100}%` }}
                                    />
                                </div>
                                <div className="text-[10px] dark:text-dark-300 text-dark-600 font-medium">
                                    {formatSize(storage.used)} / {formatSize(storage.total)} ({formatSize(storage.free)} free)
                                </div>
                            </div>
                        </div>
                    )}
                </div>
                {/* Refresh button hidden on mobile (shown next to title instead) */}
                <Button
                    onClick={() => {
                        fetchRecordings();
                        fetchStorage();
                    }}
                    variant="secondary"
                    size="sm"
                    className="hidden sm:block"
                >
                    Refresh
                </Button>
            </div>

            {/* Mobile Card View */}
            <div className="md:hidden space-y-4">
                {isLoading ? (
                    <Card className="p-8 text-center dark:text-dark-400 text-dark-600">
                        Loading recordings...
                    </Card>
                ) : sortedRecordings.length === 0 ? (
                    <Card className="p-8 text-center dark:text-dark-400 text-dark-600">
                        No recordings found.
                    </Card>
                ) : (
                    <>
                        {/* Sort Controls Mobile */}
                        <div className="flex gap-2 overflow-x-auto pb-2">
                            <button
                                onClick={() => toggleSort('date')}
                                className={`px-3 py-1.5 rounded-lg text-xs font-medium whitespace-nowrap transition-colors ${sortBy === 'date'
                                        ? 'bg-accent-red/20 text-accent-red border border-accent-red/30'
                                        : 'dark:bg-dark-800 bg-light-200 dark:text-dark-300 text-dark-600 border dark:border-dark-700 border-light-300'
                                    }`}
                            >
                                Date {sortBy === 'date' && (sortOrder === 'desc' ? '↓' : '↑')}
                            </button>
                            <button
                                onClick={() => toggleSort('name')}
                                className={`px-3 py-1.5 rounded-lg text-xs font-medium whitespace-nowrap transition-colors ${sortBy === 'name'
                                        ? 'bg-accent-red/20 text-accent-red border border-accent-red/30'
                                        : 'dark:bg-dark-800 bg-light-200 dark:text-dark-300 text-dark-600 border dark:border-dark-700 border-light-300'
                                    }`}
                            >
                                Name {sortBy === 'name' && (sortOrder === 'desc' ? '↓' : '↑')}
                            </button>
                            <button
                                onClick={() => toggleSort('size')}
                                className={`px-3 py-1.5 rounded-lg text-xs font-medium whitespace-nowrap transition-colors ${sortBy === 'size'
                                        ? 'bg-accent-red/20 text-accent-red border border-accent-red/30'
                                        : 'dark:bg-dark-800 bg-light-200 dark:text-dark-300 text-dark-600 border dark:border-dark-700 border-light-300'
                                    }`}
                            >
                                Size {sortBy === 'size' && (sortOrder === 'desc' ? '↓' : '↑')}
                            </button>
                        </div>

                        {/* Recording Cards */}
                        {sortedRecordings.map((rec) => (
                            <Card key={rec.name} className="p-4 space-y-3">
                                {/* Thumbnail + Info Row */}
                                <div className="flex gap-3">
                                    <div
                                        className="w-28 aspect-video bg-black rounded-lg border dark:border-dark-700 border-light-300 overflow-hidden cursor-pointer shrink-0"
                                        onClick={() => setSelectedVideo(rec.name)}
                                    >
                                        <img
                                            src={`${api.defaults.baseURL}/recordings/thumbnail/${encodeURIComponent(rec.name)}`}
                                            alt=""
                                            className="w-full h-full object-cover"
                                            onError={(e) => {
                                                (e.target as HTMLImageElement).style.display = 'none';
                                            }}
                                        />
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <button
                                            onClick={() => setSelectedVideo(rec.name)}
                                            className="text-sm font-medium dark:text-white text-dark-900 hover:text-accent-red transition-colors text-left line-clamp-2 break-all"
                                        >
                                            {rec.name}
                                        </button>
                                        <div className="mt-1 text-xs dark:text-dark-400 text-dark-500">
                                            {formatDate(rec.date)}
                                        </div>
                                        <div className="mt-0.5 text-xs dark:text-dark-400 text-dark-500">
                                            {formatSize(rec.size)}
                                        </div>
                                    </div>
                                </div>

                                {/* Action Buttons - Always visible on mobile */}
                                <div className="flex gap-2 pt-2 border-t dark:border-dark-700/50 border-light-300/50">
                                    <Button
                                        size="sm"
                                        variant="secondary"
                                        onClick={() => setSelectedVideo(rec.name)}
                                        className="flex-1"
                                    >
                                        Watch
                                    </Button>
                                    <Button
                                        size="sm"
                                        variant="primary"
                                        onClick={() => handleDownload(rec.name)}
                                        className="flex-1"
                                    >
                                        Download
                                    </Button>
                                    <Button
                                        size="sm"
                                        variant="danger"
                                        onClick={() => setRecordingToDelete(rec.name)}
                                        className="px-3"
                                    >
                                        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                        </svg>
                                    </Button>
                                </div>
                            </Card>
                        ))}
                    </>
                )}
            </div>

            {/* Desktop Table View */}
            <Card className="overflow-hidden hidden md:block">
                <div className="overflow-x-auto">
                    <table className="w-full text-left">
                        <thead>
                            <tr className="border-b dark:border-dark-700 border-light-300 dark:bg-dark-800/50 bg-light-200/50">
                                <th className="px-6 py-4 text-sm font-semibold dark:text-dark-300 text-dark-600">Preview</th>
                                <th
                                    className="px-6 py-4 text-sm font-semibold dark:text-dark-300 text-dark-600 cursor-pointer hover:text-white transition-colors group"
                                    onClick={() => toggleSort('name')}
                                >
                                    <div className="flex items-center gap-2">
                                        Filename
                                        <span className={`transition-opacity ${sortBy === 'name' ? 'opacity-100' : 'opacity-0 group-hover:opacity-50'}`}>
                                            {sortBy === 'name' && sortOrder === 'desc' ? '↓' : '↑'}
                                        </span>
                                    </div>
                                </th>
                                <th
                                    className="px-6 py-4 text-sm font-semibold dark:text-dark-300 text-dark-600 cursor-pointer hover:text-white transition-colors group"
                                    onClick={() => toggleSort('date')}
                                >
                                    <div className="flex items-center gap-2">
                                        Date
                                        <span className={`transition-opacity ${sortBy === 'date' ? 'opacity-100' : 'opacity-0 group-hover:opacity-50'}`}>
                                            {sortBy === 'date' && sortOrder === 'desc' ? '↓' : '↑'}
                                        </span>
                                    </div>
                                </th>
                                <th
                                    className="px-6 py-4 text-sm font-semibold dark:text-dark-300 text-dark-600 cursor-pointer hover:text-white transition-colors group"
                                    onClick={() => toggleSort('size')}
                                >
                                    <div className="flex items-center gap-2">
                                        Size
                                        <span className={`transition-opacity ${sortBy === 'size' ? 'opacity-100' : 'opacity-0 group-hover:opacity-50'}`}>
                                            {sortBy === 'size' && sortOrder === 'desc' ? '↓' : '↑'}
                                        </span>
                                    </div>
                                </th>
                                <th className="px-6 py-4 text-sm font-semibold dark:text-dark-300 text-dark-600 text-right">Actions</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y dark:divide-dark-700/50 divide-light-300/50">
                            {isLoading ? (
                                <tr>
                                    <td colSpan={5} className="px-6 py-12 text-center text-gray-500">
                                        Loading recordings...
                                    </td>
                                </tr>
                            ) : sortedRecordings.length === 0 ? (
                                <tr>
                                    <td colSpan={5} className="px-6 py-12 text-center text-gray-500">
                                        No recordings found.
                                    </td>
                                </tr>
                            ) : sortedRecordings.map((rec) => (
                                <tr key={rec.name} className="hover:bg-white/5 transition-colors group">
                                    <td className="px-6 py-4">
                                        <div
                                            className="w-24 aspect-video bg-black rounded border border-gray-800 overflow-hidden cursor-pointer"
                                            onClick={() => setSelectedVideo(rec.name)}
                                        >
                                            <img
                                                src={`${api.defaults.baseURL}/recordings/thumbnail/${encodeURIComponent(rec.name)}`}
                                                alt=""
                                                className="w-full h-full object-cover"
                                                onError={(e) => {
                                                    (e.target as HTMLImageElement).style.display = 'none';
                                                }}
                                            />
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 font-medium text-gray-200 truncate max-w-md">
                                        <button
                                            onClick={() => setSelectedVideo(rec.name)}
                                            className="hover:text-primary-400 transition-colors text-left"
                                        >
                                            {rec.name}
                                        </button>
                                    </td>
                                    <td className="px-6 py-4 text-sm text-gray-400">
                                        {formatDate(rec.date)}
                                    </td>
                                    <td className="px-6 py-4 text-sm text-gray-400">
                                        {formatSize(rec.size)}
                                    </td>
                                    <td className="px-6 py-4 opacity-0 group-hover:opacity-100 transition-opacity">
                                        <div className="flex items-center justify-end gap-3">
                                            <Button
                                                size="sm"
                                                variant="secondary"
                                                onClick={() => setSelectedVideo(rec.name)}
                                            >
                                                Watch
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="primary"
                                                onClick={() => handleDownload(rec.name)}
                                            >
                                                Download
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="danger"
                                                onClick={() => setRecordingToDelete(rec.name)}
                                            >
                                                Delete
                                            </Button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </Card>

            {/* Video Player Modal */}
            {selectedVideo && (
                <div className="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 dark:bg-black/80 bg-black/60 backdrop-blur-md animate-fade-in">
                    <div className="relative w-full max-w-5xl dark:bg-dark-900 bg-white rounded-2xl sm:rounded-3xl overflow-hidden shadow-2xl border dark:border-dark-700 border-light-300">
                        <div className="p-4 sm:p-6 border-b dark:border-dark-700 border-light-300 flex justify-between items-center">
                            <h2 className="text-base sm:text-xl font-bold dark:text-white text-dark-900 truncate pr-4">{selectedVideo}</h2>
                            <button
                                onClick={() => setSelectedVideo(null)}
                                className="p-2 dark:text-dark-300 text-dark-600 hover:text-accent-red transition-colors rounded-full dark:hover:bg-white/10 hover:bg-black/10"
                            >
                                <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                </svg>
                            </button>
                        </div>
                        <div className="aspect-video bg-black">
                            <video
                                controls
                                autoPlay
                                className="w-full h-full"
                                src={`${api.defaults.baseURL}/recordings/download/${encodeURIComponent(selectedVideo)}`}
                            >
                                Your browser does not support the video tag.
                            </video>
                        </div>
                        <div className="p-4 sm:p-6 dark:bg-dark-800/50 bg-light-100 flex justify-end">
                            <Button onClick={() => handleDownload(selectedVideo)} variant="primary">
                                Download
                            </Button>
                        </div>
                    </div>
                </div>
            )}

            {/* Delete Confirmation Modal */}
            {recordingToDelete && (
                <div className="fixed inset-0 z-[60] flex items-center justify-center p-4 dark:bg-black/80 bg-black/60 backdrop-blur-md animate-fade-in">
                    <div className="relative w-full max-w-md dark:bg-dark-900 bg-white rounded-2xl overflow-hidden shadow-2xl border dark:border-dark-700 border-light-300 p-6">
                        <h3 className="text-xl font-bold dark:text-white text-dark-900 mb-4">Delete Recording?</h3>
                        <p className="dark:text-dark-300 text-dark-600 mb-6 break-all">
                            Are you sure you want to delete <span className="dark:text-white text-dark-900 font-medium">"{recordingToDelete}"</span>? This action cannot be undone.
                        </p>
                        <div className="flex justify-end gap-3">
                            <Button
                                variant="secondary"
                                onClick={() => setRecordingToDelete(null)}
                                disabled={isDeleting}
                            >
                                Cancel
                            </Button>
                            <Button
                                variant="danger"
                                onClick={handleDelete}
                                isLoading={isDeleting}
                            >
                                Confirm Delete
                            </Button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
};

export default Recordings;
