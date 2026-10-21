export interface AuthTransport {
  readonly login: (email: string, password: string) => Promise<Response>;
  readonly refresh: (signal: AbortSignal) => Promise<Response>;
  readonly me: (token: string, signal: AbortSignal) => Promise<Response>;
  readonly send: (request: Request) => Promise<Response>;
}

export function httpAuthTransport(origin: string, fetchImplementation: typeof fetch): AuthTransport {
  return {
    login: (email, password) =>
      fetchImplementation(`${origin}/api/v1/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      }),
    refresh: (signal) => fetchImplementation(`${origin}/api/v1/auth/refresh`, { method: 'POST', signal }),
    me: (token, signal) =>
      fetchImplementation(`${origin}/api/v1/users/me`, { headers: { Authorization: `Bearer ${token}` }, signal }),
    send: (request) => fetchImplementation(request),
  };
}
