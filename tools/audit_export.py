#!/usr/bin/env python3
"""Conservative source-export checks; reports labels/paths, never matched secrets."""
import argparse
import ipaddress
from pathlib import Path
import re
import subprocess
import sys

SAFE_IPS = tuple(ipaddress.ip_network(n) for n in (
    '127.0.0.0/8', '192.0.2.0/24', '198.51.100.0/24', '203.0.113.0/24'))
PATTERNS = {
    'provider-key': r'AIza[\w-]{25,}|sk-(?:proj-)?[\w-]{20,}',
    'private-key': r'-----BEGIN [^-]*PRIVATE KEY-----',
    'bot-token': r'\b[0-9]{7,}:[A-Za-z0-9_-]{30,}',
    'home-domain': r'greathorse\.(?:ru|com)\b',
    'home-location': r'Казан[ьи]|55[.]7963|49[.]1088',
    'personal-path': r'C:\\Users\\(?:Admin|rus)\b|/home/r[u]s\b',
    'home-station-id': r'station-(?:112|187|196|251)\b',
}
FORBIDDEN_PARTS = {'private', 'snapshots', 'artifacts', 'node_modules'}
FORBIDDEN_SUFFIXES = {'.key', '.pem', '.log', '.pid', '.exe', '.dll', '.so', '.yap', '.zip', '.gz'}

def problems(name, data, deny=()):
    result = []
    path = Path(name)
    if any(part in FORBIDDEN_PARTS for part in path.parts) or path.suffix in FORBIDDEN_SUFFIXES:
        result.append('forbidden-path')
    if data.startswith((b'\x7fELF', b'MZ', b'PK\x03\x04', b'\x1f\x8b')):
        result.append('binary-or-archive')
    try:
        text = data.decode('utf-8')
    except UnicodeDecodeError:
        return result + ['non-UTF8-source']
    for label, pattern in PATTERNS.items():
        if re.search(pattern, text):
            result.append(label)
    for raw in re.findall(r'(?<![\w.])(?:\d{1,3}\.){3}\d{1,3}(?![\w.])', text):
        try:
            address = ipaddress.ip_address(raw)
        except ValueError:
            continue
        if not any(address in network for network in SAFE_IPS) and address.is_private:
            result.append('non-documentation-private-IP')
            break
    for value in deny:
        if value and value in data:
            result.append('known-private-value')
    return sorted(set(result))

def audit(root, deny=()):
    failures = []
    count = 0
    for path in sorted(root.rglob('*')):
        rel = path.relative_to(root)
        if '.git' in rel.parts:
            continue
        if path.is_symlink():
            failures.append((str(rel), ['symlink-not-allowlisted']))
        elif path.is_file():
            count += 1
            labels = problems(str(rel), path.read_bytes(), deny)
            if labels:
                failures.append((str(rel), labels))
    # Check ALL reachable blobs, including material removed by later commits.
    git = root / '.git'
    blobs = 0
    if git.exists():
        result = subprocess.run(['git', 'rev-list', '--objects', '--all'], cwd=root,
                                text=True, capture_output=True, check=True)
        for row in result.stdout.splitlines():
            fields = row.split(' ', 1)
            if len(fields) != 2:
                continue
            oid, name = fields
            kind = subprocess.check_output(['git', 'cat-file', '-t', oid], cwd=root).strip()
            if kind != b'blob':
                continue
            blobs += 1
            data = subprocess.check_output(['git', 'cat-file', 'blob', oid], cwd=root)
            labels = problems(name, data, deny)
            if labels:
                failures.append(('history:' + name, labels))
    return count, blobs, failures

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument('--deny-file', type=Path, action='append', default=[],
                        help='Private external value to compare exactly; never printed/copied')
    args = parser.parse_args()
    deny = [p.read_bytes().strip() for p in args.deny_file]
    count, blobs, failures = audit(args.root.resolve(), deny)
    for name, labels in failures:
        print('FAIL', name, ','.join(labels))
    print(f'EXPORT_AUDIT files={count} historical_blobs={blobs} failures={len(failures)}')
    return 1 if failures else 0

if __name__ == '__main__':
    sys.exit(main())
