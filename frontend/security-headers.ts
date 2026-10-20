export const contentSecurityPolicy: string = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self'",
  "font-src 'self'",
  "img-src 'self'",
  "media-src 'self'",
  "connect-src 'self'",
  "frame-src 'none'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join('; ');

export const responseHeaders: Readonly<Record<string, string>> = {
  'Referrer-Policy': 'no-referrer',
  'X-Content-Type-Options': 'nosniff',
};

export const pageHeaders: Readonly<Record<string, string>> = {
  ...responseHeaders,
  'Content-Security-Policy': contentSecurityPolicy,
};
