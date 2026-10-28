"""Tiny deployment marker; bake VERSION/FAIL_HEALTH into separate image digests."""
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        failed = self.path.endswith('/health') and os.environ.get('FAIL_HEALTH') == 'true'
        self.send_response(503 if failed else 200)
        self.end_headers()
        self.wfile.write((os.environ.get('VERSION', 'baseline') + '\n').encode())


if __name__ == '__main__':
    ThreadingHTTPServer(('0.0.0.0', 8080), Handler).serve_forever()
