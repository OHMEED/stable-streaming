import React from 'react';
import { Outlet } from 'react-router-dom';

const RequireAuth: React.FC = () => {
    // We rely on the backend API (401 Unauthorized) to handle auth redirects via api.ts interceptor.
    // This allows both LocalStorage (Header) and SSO (HttpOnly Cookie) tokens to work.
    // 
    // For local backend access: the frontend stores token in localStorage and sends via header
    // For Portal SSO access: the Portal sets an HttpOnly cookie which gets sent automatically
    // 
    // The API interceptor in api.ts handles 401 responses by redirecting to /login
    return <Outlet />;
};

export default RequireAuth;
