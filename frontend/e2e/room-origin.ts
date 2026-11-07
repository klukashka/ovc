import { readFileSync } from 'node:fs';
import { request as httpRequest } from 'node:http';
import { createServer } from 'node:https';

export interface RoomOriginServer {
  readonly stop: () => Promise<void>;
}

const roomPath = '/__test__/fake-room';

export async function startRoomOrigin(
  tls: { readonly certificate: string; readonly privateKey: string },
  port: number,
  apiPort: number,
): Promise<RoomOriginServer> {
  const server = createServer(
    { cert: readFileSync(tls.certificate), key: readFileSync(tls.privateKey) },
    (incoming, outgoing) => {
      const path = incoming.url ?? '';
      if (path !== roomPath && !path.startsWith(`${roomPath}/`)) {
        outgoing.writeHead(404).end();
        return;
      }
      const forwarded = httpRequest(
        { host: '127.0.0.1', port: apiPort, path, method: incoming.method, headers: incoming.headers },
        (answer) => {
          outgoing.writeHead(answer.statusCode ?? 502, answer.headers);
          answer.pipe(outgoing);
        },
      );
      forwarded.once('error', () => {
        outgoing.destroy();
      });
      outgoing.once('close', () => {
        forwarded.destroy();
      });
      incoming.pipe(forwarded);
    },
  );
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => {
      server.off('error', reject);
      resolve();
    });
  });
  return {
    stop: () =>
      new Promise<void>((resolve, reject) => {
        server.close((error) => {
          if (error === undefined) {
            resolve();
          } else {
            reject(error);
          }
        });
        server.closeAllConnections();
      }),
  };
}
