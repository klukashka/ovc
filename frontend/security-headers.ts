function policyWithFrames(frameSource: string): string {
  return [
    "default-src 'self'",
    "script-src 'self'",
    "style-src 'self'",
    "font-src 'self'",
    "img-src 'self'",
    "media-src 'self'",
    "connect-src 'self'",
    `frame-src ${frameSource}`,
    "object-src 'none'",
    "base-uri 'none'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join('; ');
}

export const contentSecurityPolicy: string = policyWithFrames("'none'");

const loopbackHttpsOrigin = /^https:\/\/127\.0\.0\.1:\d{1,5}$/;

export function contentSecurityPolicyWithTestRoom(roomOrigin: string): string {
  if (!loopbackHttpsOrigin.test(roomOrigin)) {
    throw new Error('The test room frame origin must be an https origin on 127.0.0.1');
  }
  return policyWithFrames(roomOrigin);
}

export const responseHeaders: Readonly<Record<string, string>> = {
  'Referrer-Policy': 'no-referrer',
  'X-Content-Type-Options': 'nosniff',
};

export const pageHeaders: Readonly<Record<string, string>> = {
  ...responseHeaders,
  'Content-Security-Policy': contentSecurityPolicy,
};

export function previewHeaders(testRoomOrigin: string | undefined): Readonly<Record<string, string>> {
  return testRoomOrigin === undefined || testRoomOrigin === ''
    ? pageHeaders
    : { ...responseHeaders, 'Content-Security-Policy': contentSecurityPolicyWithTestRoom(testRoomOrigin) };
}
