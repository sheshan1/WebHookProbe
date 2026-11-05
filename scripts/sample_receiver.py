#!/usr/bin/env python3
"""Deterministic local-only targets for the WebHookProbe sample tests."""
import argparse
import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

lock = threading.Lock()
events = {}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def respond(self, status, value, raw=False, headers=None):
        body = value.encode() if raw else json.dumps(value).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'text/plain' if raw else 'application/json')
        self.send_header('Content-Length', str(len(body)))
        for key, val in (headers or {}).items():
            self.send_header(key, val)
        self.end_headers()
        try:
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError):
            pass  # Expected when demonstrating cancellation/timeouts.

    def do_POST(self):
        try:
            size = int(self.headers.get('Content-Length', '0'))
            if size < 0 or size > 65536:
                return self.respond(413, {})
            event = json.loads(self.rfile.read(size))
            event_id = event['eventId']
            if not isinstance(event_id, str):
                raise ValueError('eventId string required')
        except (ValueError, KeyError, TypeError):
            return self.respond(400, {'error': 'eventId string required'})
        if self.path == '/error':
            return self.respond(503, {'error': 'deliberate fixture failure'})
        if self.path == '/redirect':
            return self.respond(307, {}, headers={'Location': '/webhook'})
        if self.path == '/large':
            return self.respond(200, 'x' * 5000, raw=True)
        if self.path == '/echo':
            return self.respond(200, {'authorization': self.headers.get('Authorization', '')})
        if self.path not in ('/webhook', '/slow'):
            return self.respond(404, {})
        if self.path == '/slow':
            time.sleep(5)
        with lock:
            duplicate = event_id in events
            events.setdefault(event_id, time.monotonic())
        self.respond(200, {'duplicate': duplicate})

    def do_GET(self):
        if self.path == '/health':
            return self.respond(200, {'status': 'UP'})
        kind, _, event_id = self.path.strip('/').partition('/')
        if kind == 'invalid':
            return self.respond(200, 'this is not JSON', raw=True)
        if kind == 'missing':
            return self.respond(200, {})
        if kind == 'unavailable':
            return self.respond(503, {})
        if kind == 'slow-state':
            time.sleep(5)
        with lock:
            accepted_at = events.get(event_id)
        count = int(accepted_at is not None)
        if kind == 'delayed' and accepted_at is not None:
            count = int(time.monotonic() - accepted_at >= 1.2)
        if kind == 'mismatch':
            count = 2
        if kind not in ('state', 'delayed', 'mismatch', 'slow-state'):
            return self.respond(404, {})
        self.respond(200, {'notificationCount': count})


class Server(ThreadingHTTPServer):
    request_queue_size = 128


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bind', default='127.0.0.1')
    parser.add_argument('--port', type=int, default=8090)
    args = parser.parse_args()
    server = Server((args.bind, args.port), Handler)
    server.daemon_threads = True
    print(f'Sample receiver: {args.bind}:{args.port}', flush=True)
    server.serve_forever()
