import React, { useState, useEffect, useCallback } from 'react';
import { Search, MessageSquare, Trash2, CornerDownRight, ChevronDown, ChevronUp, Send, X, Loader2 } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';

interface Comment {
    id: number;
    content: string;
    parentId: number | null;
    createdAt: string;
}

const PAGE_SIZE = 5;

export default function App() {
    const [comments, setComments] = useState<Comment[]>([]);
    const [currentPage, setCurrentPage] = useState(0);
    const [searchQuery, setSearchQuery] = useState('');
    const [hasMore, setHasMore] = useState(true);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [replyingTo, setReplyingTo] = useState<number | null>(null);
    const [newCommentContent, setNewCommentContent] = useState('');

    const fetchComments = useCallback(async (page: number, search: string) => {
        setIsLoading(true);
        setError(null);
        try {
            const offset = page * PAGE_SIZE;
            const url = `/comments?limit=${PAGE_SIZE}&offset=${offset}&search=${encodeURIComponent(search)}`;
            const response = await fetch(url);
            if (!response.ok) throw new Error('Failed to load comments');

            const rawData = await response.json();
            console.log('Raw data from Go:', rawData); // Поможет увидеть структуру в консоли

            // Пытаемся найти массив комментариев (в корне или в поле comments/data)
            const data = Array.isArray(rawData) ? rawData : (rawData.comments || rawData.data || []);

            // Нормализуем данные (Go snake_case -> TS camelCase)
            const normalizedComments = data.map((c: any) => ({
                id: c.id || c.ID || c.comment_id,
                content: c.content || c.Content || c.text || c.body || '',
                parentId: c.parentId !== undefined ? c.parentId : (c.parent_id !== undefined ? c.parent_id : (c.ParentID !== undefined ? c.ParentID : null)),
                createdAt: c.createdAt || c.created_at || c.CreatedAt || new Date().toISOString()
            }));

            setComments(normalizedComments);
            setHasMore(normalizedComments.length >= PAGE_SIZE);
        } catch (err) {
            setError(err instanceof Error ? err.message : 'An error occurred');
        } finally {
            setIsLoading(false);
        }
    }, []);

    useEffect(() => {
        fetchComments(currentPage, searchQuery);
    }, [currentPage, searchQuery, fetchComments]);

    const handleSearch = (e: React.FormEvent) => {
        e.preventDefault();
        setCurrentPage(0);
    };

    const handleDelete = async (id: number) => {
        if (!confirm(`Are you sure you want to delete comment #${id}?`)) return;
        try {
            const res = await fetch(`/comments/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error('Failed to delete comment');
            fetchComments(currentPage, searchQuery);
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to delete');
        }
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!newCommentContent.trim()) return;

        try {
            const body = {
                content: newCommentContent,
                parent_id: replyingTo
            };
            const res = await fetch('/comments', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            if (!res.ok) throw new Error('Failed to post comment');

            setNewCommentContent('');
            setReplyingTo(null);
            fetchComments(currentPage, searchQuery);
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to post');
        }
    };

    return (
        <div className="min-h-screen bg-bg font-sans flex flex-col">
            {/* Header */}
            <header className="sticky top-0 z-10 bg-white border-b border-border-main px-10 py-6 flex justify-between items-center shadow-sm">
                <div className="text-xl font-extrabold text-blue-600 tracking-tighter">COMMENTARY_PRO</div>
                <form onSubmit={handleSearch} className="relative w-80">
                    <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                        <Search className="h-4 w-4 text-slate-400" />
                    </div>
                    <input
                        type="text"
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                        placeholder="Search comments..."
                        className="block w-full pl-9 pr-3 py-2 border border-border-main rounded-lg bg-slate-50 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition-all"
                    />
                </form>
            </header>

            <main className="flex-1 max-w-[1440px] mx-auto w-full grid grid-cols-1 lg:grid-cols-[1fr_340px] gap-8 p-10 overflow-hidden">
                {/* Comments Section */}
                <div className="flex flex-col gap-4 overflow-y-auto pr-2">
                    {error && (
                        <div className="bg-red-50 border-l-4 border-red-400 p-4 rounded-lg mb-4">
                            <div className="flex">
                                <X className="h-5 w-5 text-red-400" />
                                <p className="ml-3 text-sm text-red-700">{error}</p>
                            </div>
                        </div>
                    )}

                    <div className="space-y-4 relative min-h-[300px]">
                        {/* Индикатор загрузки поверх контента, чтобы избежать резких прыжков интерфейса */}
                        {isLoading && (
                            <div className="absolute inset-0 z-20 bg-bg/40 backdrop-blur-[1px] flex justify-center items-start pt-20 rounded-xl transition-all">
                                <Loader2 className="h-8 w-8 text-blue-500 animate-spin" />
                            </div>
                        )}

                        <AnimatePresence mode="popLayout" initial={false}>
                            {comments.length === 0 && !isLoading ? (
                                <motion.div
                                    key="empty-state"
                                    initial={{ opacity: 0, scale: 0.95 }}
                                    animate={{ opacity: 1, scale: 1 }}
                                    exit={{ opacity: 0, scale: 0.95 }}
                                    className="text-center py-20 bg-white rounded-xl border border-dashed border-slate-300 shadow-sm"
                                >
                                    <MessageSquare className="mx-auto h-12 w-12 text-slate-300" />
                                    <h3 className="mt-4 text-sm font-semibold text-slate-900">No comments found</h3>
                                    <p className="mt-1 text-xs text-slate-500">Try adjusting your search or be the first to comment!</p>
                                </motion.div>
                            ) : (
                                comments.map((comment) => (
                                    <CommentNode
                                        key={comment.id}
                                        comment={comment}
                                        onReply={(id) => {
                                            setReplyingTo(id);
                                            document.getElementById('comment-form')?.scrollIntoView({ behavior: 'smooth' });
                                        }}
                                        onDelete={handleDelete}
                                    />
                                ))
                            )}
                        </AnimatePresence>
                    </div>

                    {/* Pagination */}
                    <div className="flex items-center justify-center gap-2 pt-8 mt-auto">
                        <button
                            onClick={() => setCurrentPage(prev => Math.max(0, prev - 1))}
                            disabled={currentPage === 0 || isLoading}
                            className="px-4 py-2 border border-border-main text-sm font-semibold rounded-lg text-slate-600 bg-white hover:bg-slate-50 disabled:opacity-50 transition-all"
                        >
                            Previous
                        </button>
                        <div className="flex gap-1">
                            {[...Array(3)].map((_, i) => (
                                <div
                                    key={i}
                                    className={`w-8 h-8 flex items-center justify-center rounded-lg text-sm font-medium border ${
                                        currentPage === i ? 'bg-blue-600 border-blue-600 text-white' : 'bg-white border-border-main text-slate-600'
                                    }`}
                                >
                                    {i + 1}
                                </div>
                            ))}
                        </div>
                        <button
                            onClick={() => setCurrentPage(prev => prev + 1)}
                            disabled={!hasMore || isLoading}
                            className="px-4 py-2 border border-border-main text-sm font-semibold rounded-lg text-slate-600 bg-white hover:bg-slate-50 disabled:opacity-50 transition-all"
                        >
                            Next
                        </button>
                    </div>
                </div>

                {/* Sidebar */}
                <aside className="flex flex-col gap-6">
                    {/* New Comment Form */}
                    <div id="comment-form" className="bg-white rounded-xl border border-border-main p-6 space-y-4 shadow-sm">
                        <div className="text-xs font-bold uppercase tracking-wider text-slate-400 mb-2">
                            {replyingTo ? 'Reply to Comment' : 'New Comment'}
                        </div>
                        <form onSubmit={handleSubmit} className="space-y-4">
              <textarea
                  value={newCommentContent}
                  onChange={(e) => setNewCommentContent(e.target.value)}
                  rows={6}
                  placeholder="What are your thoughts?"
                  className="block w-full px-4 py-3 border border-border-main rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 transition-all resize-none"
                  required
              />
                            <div className="flex flex-col gap-2">
                                <button
                                    type="submit"
                                    disabled={!newCommentContent.trim()}
                                    className="w-full py-2.5 px-4 bg-blue-600 text-white text-sm font-semibold rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-all"
                                >
                                    Post Comment
                                </button>
                                {replyingTo && (
                                    <button
                                        type="button"
                                        onClick={() => setReplyingTo(null)}
                                        className="w-full py-2.5 px-4 bg-transparent border border-border-main text-slate-500 text-sm font-semibold rounded-lg hover:bg-slate-50 transition-all"
                                    >
                                        Cancel Reply
                                    </button>
                                )}
                                <button
                                    type="button"
                                    className="w-full py-2.5 px-4 bg-transparent border border-border-main text-slate-500 text-sm font-semibold rounded-lg hover:bg-slate-50 transition-all"
                                >
                                    Save as Draft
                                </button>
                            </div>
                        </form>
                    </div>

                    {/* Stats Card */}
                    <div className="bg-slate-100/50 rounded-xl border border-dashed border-border-main p-6 space-y-4">
                        <div className="text-xs font-bold uppercase tracking-wider text-slate-400">Stats</div>
                        <div className="flex justify-between items-center text-sm">
                            <span className="text-slate-500 font-medium">Total Comments</span>
                            <span className="font-bold text-slate-900">{comments.length + 120}</span>
                        </div>
                        <div className="flex justify-between items-center text-sm">
                            <span className="text-slate-500 font-medium">Active Users</span>
                            <span className="font-bold text-slate-900">42</span>
                        </div>
                    </div>
                </aside>
            </main>
        </div>
    );
}

interface CommentNodeProps {
    key?: React.Key;
    comment: Comment;
    onReply: (id: number) => void;
    onDelete: (id: number) => Promise<void> | void;
    isReply?: boolean;
}

function CommentNode({ comment, onReply, onDelete, isReply = false }: CommentNodeProps): React.JSX.Element {
    const [replies, setReplies] = useState<Comment[]>([]);
    const [showReplies, setShowReplies] = useState(false);
    const [isLoadingReplies, setIsLoadingReplies] = useState(false);

    const fetchReplies = async () => {
        if (showReplies) {
            setShowReplies(false);
            return;
        }

        setIsLoadingReplies(true);
        try {
            const res = await fetch(`/comments/${comment.id}/children`);
            if (!res.ok) throw new Error('Failed to load replies');
            const rawData = await res.json();
            const data = Array.isArray(rawData) ? rawData : (rawData?.comments ?? rawData?.data ?? []);

            const normalizedReplies = data.map((c: any) => ({
                id: c.id || c.ID || c.comment_id,
                content: c.content || c.Content || c.text || c.body || '',
                parentId: c.parentId !== undefined ? c.parentId : (c.parent_id !== undefined ? c.parent_id : (c.ParentID !== undefined ? c.ParentID : null)),
                createdAt: c.createdAt || c.created_at || c.CreatedAt || new Date().toISOString()
            }));

            setReplies(normalizedReplies);
            setShowReplies(true);
        } catch (err) {
            console.error(err);
        } finally {
            setIsLoadingReplies(false);
        }
    };

    return (
        <motion.div
            layout
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95 }}
            className={`group bg-white rounded-xl border border-border-main overflow-hidden shadow-sm ${isReply ? 'ml-8 mt-3 border-l-2' : ''}`}
        >
            <div className="p-5 space-y-3">
                <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                        <div className={`h-8 w-8 rounded-full flex items-center justify-center font-bold text-[10px] ${isReply ? 'bg-blue-50 text-blue-600' : 'bg-slate-100 text-slate-500'}`}>
                            {(comment.content || '??').slice(0, 2).toUpperCase()}
                        </div>
                        <div className="flex flex-col">
                            <span className="text-sm font-semibold text-slate-900">User #{comment.id}</span>
                            <span className="text-[11px] text-slate-400 font-medium">
                • {comment.createdAt ? new Date(comment.createdAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : 'Just now'}
              </span>
                        </div>
                    </div>
                    <button
                        onClick={() => onDelete(comment.id)}
                        className="opacity-0 group-hover:opacity-100 px-2 py-1 text-red-500 hover:bg-red-50 rounded text-[11px] font-bold transition-all"
                    >
                        DELETE
                    </button>
                </div>

                <p className="text-[15px] text-slate-700 leading-relaxed">{comment.content || 'No content'}</p>

                <div className="flex items-center gap-5 pt-1">
                    <button
                        onClick={() => onReply(comment.id)}
                        className="text-xs font-bold text-blue-600 hover:underline transition-all"
                    >
                        Reply
                    </button>

                    <button
                        onClick={fetchReplies}
                        className="flex items-center gap-1.5 text-xs font-bold text-slate-400 hover:text-slate-600 transition-all"
                    >
                        {isLoadingReplies ? (
                            <Loader2 className="h-3 w-3 animate-spin" />
                        ) : showReplies ? (
                            'Hide Replies'
                        ) : (
                            'Show Replies'
                        )}
                    </button>
                </div>
            </div>

            <AnimatePresence>
                {showReplies && (
                    <motion.div
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: 'auto', opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        className="bg-slate-50/30 border-t border-slate-100 pb-4 pr-4"
                    >
                        {replies.length === 0 ? (
                            <p className="text-xs text-slate-400 italic p-5 ml-8">No replies yet.</p>
                        ) : (
                            replies.map(reply => (
                                <CommentNode
                                    key={reply.id}
                                    comment={reply}
                                    onReply={onReply}
                                    onDelete={onDelete}
                                    isReply={true}
                                />
                            ))
                        )}
                    </motion.div>
                )}
            </AnimatePresence>
        </motion.div>
    );
}
