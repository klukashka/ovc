import { isCancellationText } from './request-failures.ts';

export const redactedTextLimit = 400;
const scannedTextLimit = 8000;
const redacted = ':redacted';
const truncationMark = ' [truncated]';

const plainSegment =
  /^(?:[a-z][a-z0-9-]{0,39}|[0-9]{1,12}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[A-Za-z0-9_-]{1,40}\.(?:js|css|json|svg|png|jpg|jpeg|gif|webp|woff|woff2|ico|map|html|txt|pdf|mp4|webm))$/u;
const routeWord = /^[a-z]+(?:-[a-z]+)*$/u;
const universallyUniqueIdentifier = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;
const opaqueSegment = /^[A-Za-z0-9_-]{16,}$/u;
const tokenParentSegments = new Set(['invite', 'invitations', 'bbb', 'unsubscribe', 'verify-email', 'reset-password']);

const bearerToken = /\bbearer\s+[^\s"',;]+/giu;
const jsonWebToken = /\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}(?:\.[A-Za-z0-9_-]*)?/gu;
const emailAddress = /[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}/gu;
const secretName = /token|key|secret|password|signature|code|refresh|session|cookie/iu;
const jsonSecretName = /token|key|secret|password|signature|code|refresh|session|cookie|access/iu;
const jsonPublicName = /^code$/iu;
const apiErrorCode = /^[a-z][a-z0-9_]*$/u;
const cookieHeader = /\b((?:set-)?cookie)\s*:[^\r\n]*/giu;
const jsonStringValue = /"([\w.-]+)"\s*:\s*"((?:[^"\\]|\\.)*)"/gu;
const assignedName = /(?<![\w.-])([\w.-]+)=/gu;
const assignedValue = /[^\s&"']+/uy;
const addressOrPath = /\b[a-z][a-z0-9+.-]*:\/\/[^\s"'<>`]+|\/[^\s"'<>`]*/giu;
const opaqueToken = /[A-Za-z0-9_-]{32,}/gu;

export function redactPath(path: string): string {
  const segments = path.split('/');
  return segments
    .map((segment, index) => {
      if (segment === '') return segment;
      if (tokenParentSegments.has(segments[index - 1] ?? '')) return redacted;
      if (opaqueSegment.test(segment) && !routeWord.test(segment) && !universallyUniqueIdentifier.test(segment))
        return redacted;
      return plainSegment.test(segment) ? segment : redacted;
    })
    .join('/');
}

function redactRelativeAddress(address: string): string {
  const end = address.search(/[?#]/u);
  if (end === -1) return redactPath(address);
  return `${redactPath(address.slice(0, end))}?${redacted}`;
}

export function redactAddress(address: string): string {
  if (!URL.canParse(address)) return redactRelativeAddress(address);
  const url = new URL(address);
  const origin = url.origin !== 'null' ? url.origin : url.host === '' ? url.protocol : `${url.protocol}//${url.host}`;
  const query = url.search === '' && url.hash === '' ? '' : `?${redacted}`;
  return `${origin}${redactPath(url.pathname)}${query}`;
}

function boundedForScanning(text: string): string {
  if (text.length <= scannedTextLimit) return text;
  return text.slice(0, scannedTextLimit).replace(/\S*$/u, '');
}

function capped(text: string): string {
  if (text.length <= redactedTextLimit) return text;
  return `${text.slice(0, redactedTextLimit - truncationMark.length)}${truncationMark}`;
}

function redactJsonStringValue(match: string, name: string, value: string): string {
  if (!jsonSecretName.test(name) || (jsonPublicName.test(name) && apiErrorCode.test(value))) return match;
  return `"${name}":"${redacted}"`;
}

function redactAssignments(text: string): string {
  const names = new RegExp(assignedName.source, assignedName.flags);
  const values = new RegExp(assignedValue.source, assignedValue.flags);
  let kept = '';
  let from = 0;
  for (let found = names.exec(text); found !== null; found = names.exec(text)) {
    if (!secretName.test(found[1] ?? '')) continue;
    values.lastIndex = names.lastIndex;
    if (values.exec(text) === null) continue;
    kept += `${text.slice(from, names.lastIndex)}${redacted}`;
    from = values.lastIndex;
    names.lastIndex = values.lastIndex;
  }
  return `${kept}${text.slice(from)}`;
}

export function redactText(text: string): string {
  const withoutMarkedSecrets = boundedForScanning(text)
    .replace(cookieHeader, (_match, name: string) => `${name}: ${redacted}`)
    .replace(jsonStringValue, redactJsonStringValue)
    .replace(bearerToken, `Bearer ${redacted}`)
    .replace(jsonWebToken, redacted)
    .replace(emailAddress, redacted);
  return capped(
    redactAssignments(withoutMarkedSecrets)
      .replace(addressOrPath, (address) => redactAddress(address))
      .replace(opaqueToken, redacted),
  );
}

export function redactMethod(method: string): string {
  return /^[A-Z]{1,10}$/u.test(method) ? method : 'OTHER';
}

export type RequestFailureKind = 'cancellation' | 'other';

export interface RedactedFailedRequest {
  readonly method: string;
  readonly path: string;
  readonly failure: RequestFailureKind;
}

function pathWithoutQuery(address: string): string {
  if (URL.canParse(address)) return new URL(address).pathname;
  const end = address.search(/[?#]/u);
  return end === -1 ? address : address.slice(0, end);
}

export function redactFailedRequest(method: string, address: string, reason: string): RedactedFailedRequest {
  return {
    method: redactMethod(method),
    path: redactPath(pathWithoutQuery(address)),
    failure: isCancellationText(reason) ? 'cancellation' : 'other',
  };
}
